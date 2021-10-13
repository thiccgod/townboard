package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	cv2 "gocv.io/x/gocv"

	g "github.com/AllenDang/giu"
	l "github.com/thiccgod/townboard/lib"
)

const (
	__POWERSHELL_EXE = "powershell.exe"
	__TESSERACT_EXE  = "tesseract.exe"
	MinimumArea      = 55000
	MaximumArea      = 80000
	MinimumHeight    = 0
	MinimumWidth     = 0
	MaximumHeight    = 350
	MaximumWidth     = 300
	FirstStep        = 330
	SeqStep          = 250
	BottomRowHeight  = 259
	TopRowHeight     = 705
	ItemsPerRow      = 6
	NumColumns       = 2
	BoardSize        = 12
	CardPadding      = 50
)

var (
	__SCREENSHOT  = ""
	BoardFilePath = ""
	BoardFileName = "board.gob"
	FileName      = "s.jpg"
	FolderName    = "nwtb"
	FolderPath    = ""
	log           = l.Logger()
)

// https://gist.github.com/coolbrg/d1854cc771025efb4a30197820c2c612
type PowerShell struct {
	powerShell string
}

type void struct{}

type CardPos struct {
	X int
	Y int
}

type Task struct {
	Index    int
	Accepted bool
	Anchor   CardPos
	Data     string
	Path     string
}

type Board struct {
	Top []CardPos
	Bot []CardPos
}

// New create new session
func NewPowerShell() *PowerShell {
	ps, _ := exec.LookPath(__POWERSHELL_EXE)
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

func setupDir(init chan<- bool) *void {
	FolderPath = fmt.Sprintf("%s\\%s", os.TempDir(), FolderName)
	if _, err := os.Stat(FolderPath); os.IsNotExist(err) {
		os.MkdirAll(FolderPath, 0755)
	}
	init <- true
	return &void{}
}

// https://stackoverflow.com/questions/59996907/how-to-take-a-remote-screenshot-with-powershell
func screenshot(fileName string) string {
	return fmt.Sprintf(`
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
`, fileName)
}

func generateCardIndex() {
	f := fmt.Sprintf("%s\\%s", FolderPath, BoardFileName)
	BoardFilePath = f
	if _, err := os.Stat(f); os.IsNotExist(err) {
		RowHeights := [NumColumns]int{BottomRowHeight, TopRowHeight}
		top := []CardPos{}
		bot := []CardPos{}

		Steps := 0
		for i := 0; i < ItemsPerRow; i++ {
			if i > 0 {
				Steps += SeqStep
			} else {
				Steps += FirstStep
			}

			for j := 0; j < NumColumns; j++ {
				c := CardPos{X: Steps, Y: RowHeights[j]}
				if j == 0 {
					bot = append(bot, c)
				} else {
					top = append(top, c)
				}
			}
		}
		file, err := os.Create(f)
		defer file.Close()

		encoder := gob.NewEncoder(file)
		err = encoder.Encode(Board{Bot: bot, Top: top})

		if err != nil {
			log("%s:", err)
		}
	}
}

func processText(data Task, r chan<- string) {
	path := data.Path
	n := strings.LastIndexByte(path, '.')
	out := fmt.Sprintf("%s.txt", path[:n])

	t, _ := exec.LookPath(__TESSERACT_EXE)
	cmd := exec.Command(t, path, out)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log("%s", err)
	}
	log("%s", stdout.String())
}

func run() {
	win := cv2.NewWindow("bruh")
	img := cv2.IMRead(__SCREENSHOT, cv2.IMReadColor)
	hsv := cv2.NewMatWithSize(1920, 1080, cv2.MatTypeCV8U)
	defer hsv.Close()

	text := cv2.NewMatWithSize(1920, 1080, cv2.MatTypeCV8U)
	defer text.Close()

	cv2.CvtColor(img, &text, cv2.ColorBGRToGray)

	// cv2.Threshold(text, &text, 127.0, 255.0, cv2.ThresholdBinary)

	blurry := cv2.NewMat()
	sharp := cv2.NewMat()

	kernel := cv2.GetStructuringElement(cv2.MorphRect, image.Pt(3, 3))
	defer kernel.Close()

	cv2.GaussianBlur(text, &blurry, image.Pt(7, 7), 0, 0, cv2.BorderWrap)
	// cv2.MedianBlur(text, &text, 3)
	cv2.AddWeighted(text, 1, blurry, -0.5, 0, &sharp)
	// cv2.MorphologyEx(sharp, &sharp, cv2.MorphOpen, kernel)

	cv2.CvtColor(img, &hsv, cv2.ColorBGRToHSV)
	hsvRows, hsvCols := hsv.Rows(), hsv.Cols()
	// define HSV color upper and lower bound
	lowerMask := cv2.NewMatWithSizeFromScalar(cv2.NewScalar(16.0, 64.0, 32.0, 0.0), hsvRows, hsvCols, cv2.MatTypeCV8UC3)
	defer lowerMask.Close()
	upperMask := cv2.NewMatWithSizeFromScalar(cv2.NewScalar(64.0, 255.0, 255.0, 0.0), hsvRows, hsvCols, cv2.MatTypeCV8UC3)
	defer upperMask.Close()
	// global mask
	mask := cv2.NewMat()
	defer mask.Close()
	cv2.InRange(hsv, lowerMask, upperMask, &mask)
	hsvMask := cv2.NewMat()
	defer hsvMask.Close()
	cv2.Merge([]cv2.Mat{mask, mask, mask}, &hsvMask)
	cv2.BitwiseAnd(hsv, hsvMask, &hsv)
	cv2.CvtColor(hsv, &hsv, cv2.ColorHSVToBGR)
	cv2.CvtColor(hsv, &hsv, cv2.ColorBGRToGray)
	// c := cv2.NewCLAHEWithParams(2.0, image.Pt(1, 1))
	// c.Apply(hsv, &hsv)
	cv2.GaussianBlur(hsv, &hsv, image.Pt(3, 3), 0, 0, cv2.BorderWrap)
	cv2.AdaptiveThreshold(hsv, &hsv, 255.0, cv2.AdaptiveThresholdGaussian, cv2.ThresholdBinary, 21, 4.0)
	contours := cv2.FindContours(hsv, cv2.RetrievalTree, cv2.ChainApproxSimple)

	b := Board{}
	fmt.Println(BoardFilePath)

	boardData, _ := os.Open(BoardFilePath)
	defer boardData.Close()

	decoder := gob.NewDecoder(boardData)
	decoder.Decode(&b)

	// Quests := 0

	uniq := make(map[int]Task)

	for i := 0; i < ItemsPerRow; i++ {
		itr := i
		uniq[itr] = Task{Index: itr, Anchor: b.Bot[i], Accepted: false, Data: ""}

		itr += ItemsPerRow
		uniq[itr] = Task{Index: itr, Anchor: b.Top[i], Accepted: false, Data: ""}
	}

	r := make(chan string)
	// 0 1 2 3 4  5
	// 6 7 8 9 10 11

	for i := 0; i < contours.Size(); i++ {
		rect := cv2.BoundingRect(contours.At(i))
		area := cv2.ContourArea(contours.At(i))

		minArea := cv2.MinAreaRect(contours.At(i))
		h := minArea.Height
		w := minArea.Width

		if uint(area) > uint(MinimumArea) && uint(area) <= uint(MaximumArea) && (h > MinimumHeight && h <= MaximumHeight) && (w > MinimumWidth && w <= MaximumWidth) {

			cv2.Rectangle(&img, rect, color.RGBA{0, 0, 255, 0}, 1)
			for j := 0; j < BoardSize; j++ {
				data := uniq[j]
				if !data.Accepted {
					if (rect.Min.X >= data.Anchor.X-CardPadding && rect.Min.X <= data.Anchor.X+CardPadding) && (rect.Min.Y >= data.Anchor.Y-CardPadding && rect.Min.Y <= data.Anchor.Y+CardPadding) {
						data.Accepted = true
						cropped := sharp.Region(rect)
						res := cropped.Clone()
						f := fmt.Sprintf("%s\\%d.jpeg", FolderPath, data.Index)
						data.Path = f
						cv2.IMWrite(f, res)
						go processText(data, r)
					}
				}
				uniq[j] = data
			}
		}
	}
	for {
		win.IMShow(hsv)
		// Want to do streaming here to localhost:8080/cam
		if win.WaitKey(1) == 27 { // 27 => Esc
			break
		}
	}
}

func main() {
	wnd := g.NewMasterWindow("yuh", 400, 200, g.MasterWindowFlagsNotResizable)

	init := make(chan bool)
	go setupDir(init)

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	w := make(chan int)
	d := make(chan string)

	count := 0

	go func() {
		for {
			select {
			case <-init:
				go generateCardIndex()
			case <-c:
				log("CYA")
				os.Exit(0)
			case <-w:
				go func() {
					log("sup")
					go run()
				}()
			case <-d:
				g.Update()
			}
		}
	}()

	wnd.Run(func() {
		g.SingleWindow().Layout(
			g.Label("bruh"),
			g.Button("update").OnClick(func() {
				if len(FolderPath) > 0 {
					ps := NewPowerShell()
					__SCREENSHOT = fmt.Sprintf("%s\\%s", FolderPath, FileName)
					stdOut, stdErr, err := ps.execute(screenshot(__SCREENSHOT))
					log("\n %s \n %s \n %s ", strings.TrimSpace(stdOut), stdErr, err)
					count++
					w <- count
				}
			}),
		)
	})
}
