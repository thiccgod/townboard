package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	gocv "gocv.io/x/gocv"

	g "github.com/AllenDang/giu"
	l "github.com/thiccgod/townboard/lib"
)

const (
	__POWERSHELL_EXE = "powershell.exe"
	__TESSERACT_EXE  = "tesseract.exe"
	MinimumArea      = 55000
	MaximumArea      = 90000
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
	CurrentTown      Town
	__SCREENSHOT     = ""
	BoardFilePath    = ""
	BoardFileName    = "board.gob"
	CurrentPosition  = Point{}
	FileName         = "s.jpg"
	FolderName       = "nwtb"
	FolderPath       = ""
	log              = l.Logger()
	TreeFileName     = "tree.gob"
	TreeFilePath     = ""
	StateFileName    = "state.json"
	StateFilePath    = ""
	WorldMapFileName = "world.gob"
	WorldMapFilePath = ""
)

const (
	PositionTask = iota + 1
	BoardTask
)

type Town int64

const (
	Brightwood Town = iota
	CutlassKeys
	EbonscaleReach
	Everfall
	FirstLight
	MonarchsBluff
	Mourningdale
	Reekwater
	RestlessShore
	WeaversFen
	Windsward
)

func (t Town) String() string {
	return []string{
		"Brightwood",
		"CutlassKeys",
		"EbonscaleReach",
		"Everfall",
		"FirstLight",
		"MonarchsBluff",
		"Mourningdale",
		"Reekwater",
		"RestlessShore",
		"WeaversFen",
		"Windsward",
	}[t]
}

// https://gist.github.com/coolbrg/d1854cc771025efb4a30197820c2c612
type PowerShell struct {
	powerShell string
}

type void struct{}

type Point struct {
	X int
	Y int
}

type Task struct {
	__type    int
	Index     int
	Accepted  bool
	Anchor    Point
	Data      string
	ImagePath string
	TextPath  string
	Town      int
}

type Board struct {
	Top []Point
	Bot []Point
}

type Quest struct {
	Job      string
	Quantity int
}

type TurnIn struct {
	Location Town
	Quest    Quest
}

type State struct {
	Data []TurnIn
}

type Acc struct {
	buckets [][]TurnIn
}

var items = []TurnIn{}
var state = State{items}
var townMap = make(map[Town][]Quest)
var acc = Acc{}

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

// https://stackoverflow.com/Questions/59996907/how-to-take-a-remote-screenshot-with-powershell
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

func generateTownCoordinates() {
	f := fmt.Sprintf("%s\\%s", FolderPath, WorldMapFileName)
	WorldMapFilePath = f
	if _, err := os.Stat(f); os.IsNotExist(err) {
		WorldMap := make(map[Point]Town)

		WorldMap[Point{X: 8000, Y: 2000}] = CutlassKeys
		WorldMap[Point{X: 8000, Y: 4000}] = MonarchsBluff
		WorldMap[Point{X: 8000, Y: 6000}] = EbonscaleReach
		WorldMap[Point{X: 9000, Y: 5000}] = Everfall
		WorldMap[Point{X: 9000, Y: 1000}] = FirstLight
		WorldMap[Point{X: 10000, Y: 3000}] = Windsward
		WorldMap[Point{X: 10000, Y: 7000}] = Brightwood
		WorldMap[Point{X: 11000, Y: 4000}] = Reekwater
		WorldMap[Point{X: 12000, Y: 6000}] = WeaversFen
		WorldMap[Point{X: 13000, Y: 5000}] = RestlessShore
		WorldMap[Point{X: 14000, Y: 7000}] = Mourningdale

		file, err := os.Create(f)
		defer file.Close()

		encoder := gob.NewEncoder(file)
		err = encoder.Encode(WorldMap)

		if err != nil {
			log("%s:", err)
		}
	}
}

func buildTownMap(s *State) map[Town][]Quest {
	var local = make(map[Town][]Quest)
	for _, x := range s.Data {
		if _, ok := local[x.Location]; ok {
			local[x.Location] = append(local[x.Location], x.Quest)
		} else {
			local[x.Location] = []Quest{x.Quest}
		}
	}
	return local
}

func loadState(ch chan<- Acc) {
	f := fmt.Sprintf("%s\\%s", FolderPath, StateFileName)
	StateFilePath = f
	if _, err := os.Stat(f); os.IsNotExist(err) {
		file, _ := os.Create(f)
		defer file.Close()
		res, _ := json.MarshalIndent(state, "", " ")
		os.WriteFile(StateFilePath, res, 0644)
		file.Sync()
	} else if err == nil {
		file, _ := os.OpenFile(StateFilePath, os.O_RDONLY, 0644)
		s := State{}
		prev, _ := os.ReadFile(file.Name())
		json.Unmarshal(prev, &s)
		acc = *generateAggregate(s)
		ch <- acc
	}
}

func purge(t Town) []TurnIn {
	purged := []TurnIn{}
	for i, x := range state.Data {
		if state.Data[i].Location != t {
			purged = append(purged, x)
		}
	}
	return purged
}

func generateAggregate(s State) *Acc {
	if len(s.Data) > 0 {
		shared := make(map[string][]TurnIn)
		for _, x := range s.Data {
			if _, ok := shared[x.Quest.Job]; ok {
				shared[x.Quest.Job] = append(shared[x.Quest.Job], x)
			} else {
				shared[x.Quest.Job] = []TurnIn{x}
			}
		}
		var a [][]TurnIn
		for _, v := range shared {
			a = append(a, v)
		}
		fmt.Println(a)
		acc = Acc{buckets: a}
		return &acc
	}
	return &Acc{}
}

func saveState(localState State) *State {
	fmt.Println("STATE SHOULD BE EMPTY", state)
	if !reflect.DeepEqual(localState, state) || len(state.Data) == 0 {
		if _, err := os.Stat(StateFilePath); err == nil {
			file, _ := os.OpenFile(StateFilePath, os.O_TRUNC, 0644)
			defer file.Close()
			tmp := State{Data: append(localState.Data[:], state.Data[:]...)}

			uniqueMap := make(map[TurnIn]bool)
			unique := []TurnIn{}

			for _, v := range tmp.Data {
				if _, exist := uniqueMap[v]; !exist {
					uniqueMap[v] = true
					unique = append(unique, v)
				} else {
					uniqueMap[v] = false
				}
			}
			tmp.Data = unique

			res, _ := json.MarshalIndent(tmp, "", " ")
			os.WriteFile(StateFilePath, res, 0644)
			file.Sync()
			return &tmp
		} else if err != nil {
			log("%s", err)
		}
	}
	return &State{}
}

func generateCardIndex() {
	f := fmt.Sprintf("%s\\%s", FolderPath, BoardFileName)
	BoardFilePath = f
	if _, err := os.Stat(f); os.IsNotExist(err) {
		RowHeights := [NumColumns]int{BottomRowHeight, TopRowHeight}
		top := []Point{}
		bot := []Point{}

		Steps := 0
		for i := 0; i < ItemsPerRow; i++ {
			if i > 0 {
				Steps += SeqStep
			} else {
				Steps += FirstStep
			}

			for j := 0; j < NumColumns; j++ {
				c := Point{X: Steps, Y: RowHeights[j]}
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

func tesseract(path string) string {
	config := "-c"
	ConfigOptions := "tessedit_char_whitelist=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 "

	dpi := "--dpi"
	DpiOptions := "300"
	t, _ := exec.LookPath(__TESSERACT_EXE)
	args := []string{path, "stdout", config, ConfigOptions, dpi, DpiOptions}
	cmd := exec.Command(t, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log("%s", cmd.Stderr)
		log("%s", err)
	}

	clean := regexp.MustCompile(`\r?\n`)
	s := clean.ReplaceAllString(stdout.String(), " ")
	return s
}

func handleData(path string, ch chan<- Quest) {
	s := tesseract(path)
	MatchTaskRegex := regexp.MustCompile(`([A-Z][a-z].*) ([A-Z]{1}[a-z]{1}\w+)`)
	matches := MatchTaskRegex.FindAllString(s, -1)
	data := strings.Join(matches[:], " ")
	ExtractNumRegex := regexp.MustCompile("[0-9]+")
	nums := ExtractNumRegex.FindString(data)
	Job := strings.ReplaceAll(data, nums, "")
	Job = strings.ReplaceAll(Job, "  ", " ")
	Quantity, _ := strconv.Atoi(nums)
	ch <- Quest{Job, Quantity}
}

func handlePosition(path string, ch chan<- Town) {
	s := tesseract(path)
	MatchPositionRegex := regexp.MustCompile(`(Position \d+\s+\d+\s\d+)`)
	pos := MatchPositionRegex.FindString(s)
	var PositionPrecision float64 = 1000.000
	var Round float64 = 1000.0
	ExtractCoordinatesRegex := regexp.MustCompile("[0-9]+")
	matches := ExtractCoordinatesRegex.FindAllString(pos, -1)
	var x, y float64
	for i := 0; i < len(matches)-1; i++ {
		coord, _ := strconv.ParseFloat(matches[i], 3)
		c := math.Ceil(float64(coord/PositionPrecision)/Round) * Round
		if i == 0 {
			x = c
		} else {
			y = c
		}
	}
	CurrentPosition = Point{X: int(x), Y: int(y)}
	log("current pos %s", CurrentPosition)

	w := make(map[Point]Town)
	mapData, _ := os.Open(WorldMapFilePath)
	defer mapData.Close()

	decoder := gob.NewDecoder(mapData)
	decoder.Decode(&w)

	fmt.Println(CurrentPosition.X, CurrentPosition.Y)
	if !(CurrentPosition.X == 0 && CurrentPosition.Y == 0) {
		ch <- w[CurrentPosition]
	}
}

func cleanup(path string) {
	contents, err := filepath.Glob(path)
	if err != nil {
		return
	}
	for _, item := range contents {
		err = os.RemoveAll(item)
		log("%s", err)
	}
}

func run(ch chan<- Acc) {
	pc := make(chan Town)
	dc := make(chan Quest, NumColumns*ItemsPerRow)

	img := gocv.IMRead(__SCREENSHOT, gocv.IMReadColor)

	text := gocv.NewMatWithSize(1920, 1080, gocv.MatTypeCV8U)
	defer text.Close()

	// text pre-processing
	gocv.CvtColor(img, &text, gocv.ColorBGRToGray)
	blurry := gocv.NewMat()
	defer blurry.Close()
	sharp := gocv.NewMat()
	defer sharp.Close()
	gocv.GaussianBlur(text, &blurry, image.Pt(1, 1), 0, 0, gocv.BorderWrap)
	gocv.AddWeighted(text, 2, blurry, 0, 0, &sharp)
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5))
	defer kernel.Close()
	m := gocv.NewMat()
	defer m.Close()
	mInv := gocv.NewMat()
	defer mInv.Close()
	gocv.InRange(sharp,
		gocv.NewMatFromScalar(gocv.NewScalar(0.0, 0.0, 0.0, 0.0), gocv.MatTypeCV8UC3),
		gocv.NewMatFromScalar(gocv.NewScalar(200.0, 200.0, 200.0, 0.0), gocv.MatTypeCV8UC3),
		&m)
	gocv.BitwiseNot(m, &mInv)
	gocv.BitwiseAnd(sharp, mInv, &sharp)

	// contour finding
	hsv := gocv.NewMatWithSize(1920, 1080, gocv.MatTypeCV8U)
	defer hsv.Close()
	gocv.CvtColor(img, &hsv, gocv.ColorBGRToHSV)
	hsvRows, hsvCols := hsv.Rows(), hsv.Cols()
	lowerMask := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(16.0, 64.0, 32.0, 0.0), hsvRows, hsvCols, gocv.MatTypeCV8UC3)
	defer lowerMask.Close()
	upperMask := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(64.0, 255.0, 255.0, 0.0), hsvRows, hsvCols, gocv.MatTypeCV8UC3)
	defer upperMask.Close()
	mask := gocv.NewMat()
	defer mask.Close()
	gocv.InRange(hsv, lowerMask, upperMask, &mask)
	hsvMask := gocv.NewMat()
	defer hsvMask.Close()
	gocv.Merge([]gocv.Mat{mask, mask, mask}, &hsvMask)
	gocv.BitwiseAnd(hsv, hsvMask, &hsv)
	gocv.CvtColor(hsv, &hsv, gocv.ColorHSVToBGR)
	gocv.CvtColor(hsv, &hsv, gocv.ColorBGRToGray)
	c := gocv.NewCLAHEWithParams(2.0, image.Pt(1, 1))
	c.Apply(hsv, &hsv)
	gocv.GaussianBlur(hsv, &hsv, image.Pt(3, 3), 0, 0, gocv.BorderWrap)
	gocv.AdaptiveThreshold(hsv, &hsv, 255.0, gocv.AdaptiveThresholdGaussian, gocv.ThresholdBinary, 15, 4.0)
	contours := gocv.FindContours(hsv, gocv.RetrievalTree, gocv.ChainApproxSimple)

	ResolutionWidth := 1920
	ResolutionHeight := 19
	TargetWidth := 1420
	TargetHeight := 35

	position := gocv.NewMat()
	defer position.Close()

	const ResizedPositionHeight = 80
	const ResizedPositionWidth = 1400

	roi := image.Rect(TargetWidth, TargetHeight, ResolutionWidth, ResolutionHeight)
	pos := sharp.Region(roi)
	gocv.Resize(pos, &position, image.Pt(ResizedPositionWidth, ResizedPositionHeight), 0, 0, gocv.InterpolationArea)
	f := fmt.Sprintf("%s\\%s.jpeg", FolderPath, "position")
	gocv.IMWrite(f, position)
	p := Task{__type: PositionTask, ImagePath: f}
	go handlePosition(p.ImagePath, pc)

	b := Board{}
	boardData, _ := os.Open(BoardFilePath)
	defer boardData.Close()

	decoder := gob.NewDecoder(boardData)
	decoder.Decode(&b)

	uniq := make(map[int]Task)

	for i := 0; i < ItemsPerRow; i++ {
		itr := i
		uniq[itr] = Task{__type: BoardTask, Index: itr, Anchor: b.Bot[i], Accepted: false, Data: ""}

		itr += ItemsPerRow
		uniq[itr] = Task{__type: BoardTask, Index: itr, Anchor: b.Top[i], Accepted: false, Data: ""}
	}

	// 0 1 2 3 4  5
	// 6 7 8 9 10 11

	for i := 0; i < contours.Size(); i++ {
		rect := gocv.BoundingRect(contours.At(i))
		area := gocv.ContourArea(contours.At(i))

		minArea := gocv.MinAreaRect(contours.At(i))
		h := minArea.Height
		w := minArea.Width

		if uint(area) > uint(MinimumArea) && uint(area) <= uint(MaximumArea) && (h > MinimumHeight && h <= MaximumHeight) && (w > MinimumWidth && w <= MaximumWidth) {
			gocv.Rectangle(&img, rect, color.RGBA{0, 0, 255, 0}, 1)

			index := int(rect.Min.X/SeqStep) - 1
			// 0 top, $ItemsPerRow bottom
			offset := 0
			isBottomRow := (rect.Min.Y <= BottomRowHeight+CardPadding)
			if !isBottomRow {
				offset = ItemsPerRow
			}
			itr := index + offset
			data := uniq[index+offset]
			if !data.Accepted {
				if (rect.Min.X >= data.Anchor.X-CardPadding && rect.Min.X <= data.Anchor.X+CardPadding) && (rect.Min.Y >= data.Anchor.Y-CardPadding && rect.Min.Y <= data.Anchor.Y+CardPadding) {
					data.Accepted = true
					cropped := sharp.Region(rect)
					res := cropped.Clone()
					f := fmt.Sprintf("%s\\%d.jpeg", FolderPath, data.Index)
					data.ImagePath = f
					gocv.IMWrite(f, res)
				}
			}
			uniq[itr] = data
		}
	}
	count := 0
	localState := State{}
	for _, x := range uniq {
		if x.Accepted {
			count++
			go handleData(x.ImagePath, dc)
		}
	}
	for {
		select {
		case m1 := <-pc:
			CurrentTown = m1
		case m2 := <-dc:
			{
				os.Remove(__SCREENSHOT)
				t := TurnIn{Location: CurrentTown, Quest: m2}
				localState.Data = append(localState.Data, t)
				if len(localState.Data) == count {
					purged := purge(CurrentTown)
					state.Data = purged
					townMap = buildTownMap(&localState)
					state = *saveState(localState)
					acc = *generateAggregate(state)
					fmt.Println("ACC", acc)
					ch <- acc
				}
			}
		}
	}
}

func buildLabel(text string) *g.TreeTableRowWidget {
	return g.TreeTableRow(text)
}

func buildDataTree() *g.TreeTableWidget {
	f := fmt.Sprintf("%s\\%s", FolderPath, TreeFileName)
	TreeFilePath = f
	prev := []*g.TreeTableRowWidget{}

	if _, err := os.Stat(TreeFilePath); os.IsNotExist(err) {
		file, _ := os.Create(f)
		defer file.Close()
	} else if err == nil {
		prevTree, _ := os.Open(f)
		defer prevTree.Close()

		decoder := gob.NewDecoder(prevTree)
		decoder.Decode(&prev)
	}

	cur := g.TreeTableWidget{}
	leaves := []*g.TreeTableRowWidget{}
	for _, x := range acc.buckets {
		nodes := []*g.TreeTableRowWidget{}
		totalQuantity := 0
		locations := []Town{}
		quantities := []int{}
		for _, y := range x {
			locations = append(locations, y.Location)
			quantities = append(quantities, y.Quest.Quantity)
			nodes = append(nodes, buildLabel(fmt.Sprintf("%s %d", y.Location, y.Quest.Quantity)))
		}
		for _, v := range quantities {
			totalQuantity += v
		}
		job := fmt.Sprintf("%s %d", x[0].Quest.Job, totalQuantity)
		leaves = append(leaves, g.TreeTableRow(job).Children(nodes...))
	}

	rows := []*g.TreeTableRowWidget{}
	if !reflect.DeepEqual(prev, []*g.TreeTableRowWidget{}) {
		rows = append(prev[:], leaves[:]...)
	} else {
		rows = leaves
	}
	cur.Columns(g.TableColumn("townboard")).Rows(rows...)
	treeFile, _ := os.OpenFile(TreeFilePath, os.O_RDONLY, 0644)
	encoder := gob.NewEncoder(treeFile)
	encoder.Encode(leaves)

	return cur.Columns(g.TableColumn("townboard")).Rows(leaves...)
}

func main() {
	wnd := g.NewMasterWindow("yuh", 1024, 768, g.MasterWindowFlagsNotResizable)

	init := make(chan bool)
	go setupDir(init)

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	u := make(chan Acc)
	w := make(chan int)

	count := 0

	go func() {
		for {
			select {
			case <-init:
				{
					go loadState(u)
					go generateCardIndex()
					go generateTownCoordinates()
				}
			case <-c:
				log("CYA")
				os.Exit(0)
			case <-w:
				go func() {
					log("sup")
					go run(u)
				}()
			case msg := <-u:
				{
					// cleanup(fmt.Sprintf("%s\\*.jpeg", FolderPath))
					fmt.Println("HERE", msg)
				}
			}
		}
	}()

	wnd.Run(func() {
		g.SingleWindow().Layout(
			buildDataTree(),
				g.Button("update").OnClick(func() {
					if len(FolderPath) > 0 {
						ps := NewPowerShell()
						__SCREENSHOT = fmt.Sprintf("%s\\%s", FolderPath, FileName)
						stdOut, stdErr, err := ps.execute(screenshot(__SCREENSHOT))
						log("\n %s \n %s \n %s ", strings.TrimSpace(stdOut), stdErr, err)
						count++
						w <- count
					}
				})),
	})
}
