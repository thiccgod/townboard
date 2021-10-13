package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"unsafe"

	l "github.com/thiccgod/townboard/lib"
)

const (
    ModAlt = 1 << iota
    ModCtrl
    ModShift
    ModWin
)

type Hotkey struct {
    Id        int // Unique id
    Modifiers int // Mask of modifiers
    KeyCode   int // Key code, e.g. 'A'
}

func (h *Hotkey) String() string {
    mod := &bytes.Buffer{}
    if h.Modifiers&ModAlt != 0 {
        mod.WriteString("Alt+")
    }
    if h.Modifiers&ModCtrl != 0 {
        mod.WriteString("Ctrl+")
    }
    if h.Modifiers&ModShift != 0 {
        mod.WriteString("Shift+")
    }
    if h.Modifiers&ModWin != 0 {
        mod.WriteString("Win+")
    }
    return fmt.Sprintf("Hotkey[Id: %d, %s%c]", h.Id, mod, h.KeyCode)
}


var log = l.Logger()

// https://gist.github.com/coolbrg/d1854cc771025efb4a30197820c2c612
type PowerShell struct {
	powerShell string
}

// New create new session
func NewPowerShell() *PowerShell {
	ps, _ := exec.LookPath("powershell.exe")
	return &PowerShell{
		powerShell: ps,
	}
}

func (p *PowerShell) execute(args ...string) (stdOut string, stdErr string, err error) {
	args = append([]string{"-NoProfile", "-NonInteractive"}, args...)
	cmd := exec.Command(p.powerShell, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	stdOut, stdErr = stdout.String(), stderr.String()
	return
}


var filename = "tb-screenshot.jpg"
var file = fmt.Sprintf("%s\\%s", os.TempDir(), filename)

// https://stackoverflow.com/questions/59996907/how-to-take-a-remote-screenshot-with-powershell
var screenshot = fmt.Sprintf(`
$img="%s"

Add-Type -AssemblyName System.Windows.Forms
$screens=[System.Windows.Forms.Screen]::AllScreens

Add-Type -AssemblyName System.Drawing
function screenshot($s) {
    $w=$s.Bounds.Width
    $h=$s.Bounds.Height
    $x=$s.Bounds.X
    $y=$s.Bounds.Y

    $b=new-object System.Drawing.Bitmap $w, $h
    $g=[System.Drawing.Graphics]::FromImage($b)
    $g.CopyFromScreen($x, $y, 0, 0, $b.Size)

    $b.Save($img)
}

foreach ($screen in $screens)
{
    if ($screen.Primary -eq $TRUE ) {
        screenshot($screen)
    }
}
`, file)

type MSG struct {
    HWND   uintptr
    UINT   uintptr
    WPARAM int16
    LPARAM int64
    DWORD  int32
    POINT  struct{ X, Y int64 }
}

func main() {

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	user32 := syscall.MustLoadDLL("user32")
	defer user32.Release()

	reghotkey := user32.MustFindProc("RegisterHotKey")
	keys := map[int16]*Hotkey{
		1: &Hotkey{1, ModCtrl + ModShift, 'P'},  // CTRL+SHIFT+P
	}
	for _, v := range keys {
		r1, _, err := reghotkey.Call(
			0, uintptr(v.Id), uintptr(v.Modifiers), uintptr(v.KeyCode))
		if r1 == 1 {
			fmt.Println("Registered", v)
		} else {
			fmt.Println("Failed to register", v, ", error:", err)
		}
	}

	ps := NewPowerShell()

	go func() {
		<-c
		log("EXITING")
		os.Exit(1)
	}()

	for {
		getmsg := user32.MustFindProc("GetMessageW")
		var msg = &MSG{}
		getmsg.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
		if id := msg.WPARAM; id != 0 {
			fmt.Println("Hotkey pressed:", keys[id])
			if id == 1 { 
				stdOut, stdErr, err := ps.execute(screenshot)
				log(fmt.Sprintf("\n %s \n %s \n %s ", strings.TrimSpace(stdOut), stdErr, err ))
			}
		}
	}
}