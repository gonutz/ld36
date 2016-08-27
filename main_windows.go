package main

//#include <Windows.h>
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"

	"github.com/AllenDang/w32"
	"github.com/gonutz/blob"
	"github.com/gonutz/d3d9"
	"github.com/gonutz/mixer"
	"github.com/gonutz/payload"

	"github.com/gonutz/ld36/game"
)

func init() {
	runtime.LockOSThread()
}

const (
	version = "1"

	vertexFormat = d3d9.FVF_XYZRHW | d3d9.FVF_DIFFUSE | d3d9.FVF_TEX1
	vertexStride = 28
)

var (
	readFile          func(id string) ([]byte, error) = readFileFromDisk
	rscBlob           *blob.Blob
	logFile           io.WriteCloser
	muted             bool
	previousPlacement C.WINDOWPLACEMENT
	device            d3d9.Device
	windowW, windowH  int
	events            []game.InputEvent
)

func main() {
	// close the log file at the end of the program
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	defer func() {
		if err := recover(); err != nil {
			logf("panic: %v\nstack\n---\n%s\n---\n", err, debug.Stack())
			msg := fmt.Sprint("panic: ", err)
			const MB_TOPMOST = 0x00040000
			w32.MessageBox(0, msg, "Error", w32.MB_OK|w32.MB_ICONERROR|MB_TOPMOST)
			//fatal("panic: ", err)
		}
	}()

	// load the resource blob from the executable
	rscBlobData, err := payload.Read()
	if err == nil {
		rscBlob, err = blob.Read(bytes.NewReader(rscBlobData))
		if err == nil {
			readFile = readFileFromBlob
			logf("blob in exe contains %v item(s)\n", rscBlob.ItemCount())
		} else {
			logln("unable to decode blob: ", err)
		}
	} else {
		logln("unable to read payload:", err)
	}

	// create the window and initialize DirectX
	w32Window, err := openWindow(
		"LD36WindowClass",
		handleMessage,
		0, 0, 640, 480,
	)
	if err != nil {
		fatal("unable to open window: ", err)
	}
	cWindow := C.HWND(unsafe.Pointer(w32Window))
	w32.SetWindowText(w32Window, "LD36 - v"+version)
	fullscreen := true
	//fullscreen = false // NOTE toggle comment on this line for debugging
	if fullscreen {
		toggleFullscreen(cWindow)
	}
	client := w32.GetClientRect(w32Window)
	windowW = int(client.Right - client.Left)
	windowH = int(client.Bottom - client.Top)

	err = mixer.Init()
	if err != nil {
		logln("unable to initialize the DirectSound8 mixer: ", err)
		muted = true
	} else {
		defer mixer.Close()
	}

	// initialize Direct3D9
	if err := d3d9.Init(); err != nil {
		fatal("unable to initialize Direct3D9: ", err)
	}
	defer d3d9.Close()

	d3d, err := d3d9.Create(d3d9.SDK_VERSION)
	if err != nil {
		fatal("unable to create Direct3D9 object: ", err)
	}
	defer d3d.Release()

	var maxScreenW, maxScreenH uint
	for i := uint(0); i < d3d.GetAdapterCount(); i++ {
		mode, err := d3d.GetAdapterDisplayMode(i)
		if err == nil {
			if mode.Width > maxScreenW {
				maxScreenW = mode.Width
			}
			if mode.Height > maxScreenH {
				maxScreenH = mode.Height
			}
		}
	}
	if maxScreenW == 0 || maxScreenH == 0 {
		maxScreenW, maxScreenH = uint(windowW), uint(windowH)
	}

	var createFlags uint32 = d3d9.CREATE_SOFTWARE_VERTEXPROCESSING
	caps, err := d3d.GetDeviceCaps(d3d9.ADAPTER_DEFAULT, d3d9.DEVTYPE_HAL)
	if err == nil &&
		caps.DevCaps&d3d9.DEVCAPS_HWTRANSFORMANDLIGHT != 0 {
		createFlags = d3d9.CREATE_HARDWARE_VERTEXPROCESSING
		logln("graphics card supports hardware vertex processing")
	}

	device, _, err = d3d.CreateDevice(
		d3d9.ADAPTER_DEFAULT,
		d3d9.DEVTYPE_HAL,
		unsafe.Pointer(cWindow),
		createFlags,
		d3d9.PRESENT_PARAMETERS{
			BackBufferWidth:      maxScreenW,
			BackBufferHeight:     maxScreenH,
			BackBufferFormat:     d3d9.FMT_A8R8G8B8,
			BackBufferCount:      1,
			PresentationInterval: d3d9.PRESENT_INTERVAL_ONE, // enable VSync
			Windowed:             true,
			SwapEffect:           d3d9.SWAPEFFECT_DISCARD,
			HDeviceWindow:        unsafe.Pointer(cWindow),
		},
	)
	if err != nil {
		fatal("unable to create Direct3D9 device: ", err)
	}
	defer device.Release()

	device.SetFVF(vertexFormat)
	device.SetRenderState(d3d9.RS_ZENABLE, d3d9.ZB_FALSE)
	//device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_CCW)
	// TODO remove this once everything is drawn in the right order
	device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_NONE)
	device.SetRenderState(d3d9.RS_LIGHTING, 0)
	device.SetRenderState(d3d9.RS_SRCBLEND, d3d9.BLEND_SRCALPHA)
	device.SetRenderState(d3d9.RS_DESTBLEND, d3d9.BLEND_INVSRCALPHA)
	device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 1)
	// texture filter for when zooming
	device.SetSamplerState(0, d3d9.SAMP_MINFILTER, d3d9.TEXF_LINEAR)
	device.SetSamplerState(0, d3d9.SAMP_MAGFILTER, d3d9.TEXF_LINEAR)

	device.SetTextureStageState(0, d3d9.TSS_COLOROP, d3d9.TOP_MODULATE)
	device.SetTextureStageState(0, d3d9.TSS_COLORARG1, d3d9.TA_TEXTURE)
	device.SetTextureStageState(0, d3d9.TSS_COLORARG2, d3d9.TA_DIFFUSE)

	device.SetTextureStageState(0, d3d9.TSS_ALPHAOP, d3d9.TOP_MODULATE)
	device.SetTextureStageState(0, d3d9.TSS_ALPHAARG1, d3d9.TA_TEXTURE)
	device.SetTextureStageState(0, d3d9.TSS_ALPHAARG2, d3d9.TA_DIFFUSE)

	device.SetTextureStageState(1, d3d9.TSS_COLOROP, d3d9.TOP_DISABLE)
	device.SetTextureStageState(1, d3d9.TSS_ALPHAOP, d3d9.TOP_DISABLE)

	res := newGameResources()
	defer res.close()
	g := game.New(res)

	var msg C.MSG
	C.PeekMessage(&msg, nil, 0, 0, C.PM_NOREMOVE)
	for msg.message != C.WM_QUIT {
		if C.PeekMessage(&msg, nil, 0, 0, C.PM_REMOVE) != 0 {
			C.TranslateMessage(&msg)
			C.DispatchMessage(&msg)
		} else {
			device.SetViewport(
				d3d9.VIEWPORT{0, 0, uint32(windowW), uint32(windowH), 0, 1},
			)
			device.Clear(nil, d3d9.CLEAR_TARGET, d3d9.ColorRGB(0, 95, 83), 1, 0)
			device.BeginScene()

			g.Frame(events)
			events = events[0:0]

			device.EndScene()
			// TODO handle device lost error
			device.Present(
				&d3d9.RECT{0, 0, int32(windowW), int32(windowH)},
				nil,
				nil,
				nil,
			)
		}
	}
}

func addEvent(key game.Key, down bool) {
	events = append(events, game.InputEvent{
		Key:  key,
		Down: down,
	})
}

func handleMessage(window w32.HWND, message uint32, w, l uintptr) uintptr {
	switch message {
	case w32.WM_KEYUP:
		switch w {
		case w32.VK_LEFT:
			addEvent(game.KeyLeft, false)
		case w32.VK_RIGHT:
			addEvent(game.KeyRight, false)
		}
		return 1
	case w32.WM_KEYDOWN:
		switch w {
		case w32.VK_LEFT:
			addEvent(game.KeyLeft, true)
		case w32.VK_RIGHT:
			addEvent(game.KeyRight, true)
		case w32.VK_ESCAPE:
			w32.SendMessage(window, w32.WM_CLOSE, 0, 0)
		case w32.VK_F11:
			toggleFullscreen((C.HWND)(unsafe.Pointer(window)))
		}
		return 1
	case w32.WM_DESTROY:
		w32.PostQuitMessage(0)
		return 1
	case C.WM_SIZE:
		windowW, windowH = int((uint(l))&0xFFFF), int((uint(l)>>16)&0xFFFF)
		return 1
	default:
		return w32.DefWindowProc(window, message, w, l)
	}
}

type messageCallback func(window w32.HWND, msg uint32, w, l uintptr) uintptr

func openWindow(
	className string,
	callback messageCallback,
	x, y, width, height int,
) (
	w32.HWND, error,
) {
	windowProc := syscall.NewCallback(callback)

	class := w32.WNDCLASSEX{
		Size:      C.sizeof_WNDCLASSEX,
		WndProc:   windowProc,
		Cursor:    w32.LoadCursor(0, (*uint16)(unsafe.Pointer(uintptr(w32.IDC_ARROW)))),
		ClassName: syscall.StringToUTF16Ptr(className),
	}

	atom := w32.RegisterClassEx(&class)
	if atom == 0 {
		return 0, errors.New("RegisterClassEx failed")
	}

	window := w32.CreateWindowEx(
		0,
		syscall.StringToUTF16Ptr(className),
		nil,
		w32.WS_OVERLAPPED|w32.WS_CAPTION|w32.WS_SYSMENU|w32.WS_VISIBLE,
		x, y, width, height,
		0, 0, 0, nil,
	)
	if window == 0 {
		return 0, errors.New("CreateWindowEx failed")
	}

	return window, nil
}

func toggleFullscreen(window C.HWND) {
	style := C.GetWindowLong(window, C.GWL_STYLE)
	if style&C.WS_OVERLAPPEDWINDOW != 0 {
		// go into full-screen
		monitorInfo := C.MONITORINFO{cbSize: C.sizeof_MONITORINFO}
		previousPlacement.length = C.sizeof_WINDOWPLACEMENT
		monitor := C.MonitorFromWindow(window, C.MONITOR_DEFAULTTOPRIMARY)
		if C.GetWindowPlacement(window, &previousPlacement) != 0 &&
			C.GetMonitorInfo(monitor, &monitorInfo) != 0 {
			C.SetWindowLong(
				window,
				C.GWL_STYLE,
				style & ^C.WS_OVERLAPPED & ^w32.WS_CAPTION & ^w32.WS_SYSMENU,
			)
			C.SetWindowPos(window, C.HWND(unsafe.Pointer(uintptr(0))),
				C.int(monitorInfo.rcMonitor.left),
				C.int(monitorInfo.rcMonitor.top),
				C.int(monitorInfo.rcMonitor.right-monitorInfo.rcMonitor.left),
				C.int(monitorInfo.rcMonitor.bottom-monitorInfo.rcMonitor.top),
				C.SWP_NOOWNERZORDER|C.SWP_FRAMECHANGED,
			)
		}
		C.ShowCursor(0)
	} else {
		// go into windowed mode
		C.SetWindowLong(
			window,
			C.GWL_STYLE,
			style|w32.WS_OVERLAPPED|w32.WS_CAPTION|w32.WS_SYSMENU,
		)
		C.SetWindowPlacement(window, &previousPlacement)
		C.SetWindowPos(window, nil, 0, 0, 0, 0,
			C.SWP_NOMOVE|C.SWP_NOSIZE|C.SWP_NOZORDER|
				C.SWP_NOOWNERZORDER|C.SWP_FRAMECHANGED,
		)
		C.ShowCursor(1)
	}
}

func readFileFromDisk(filename string) ([]byte, error) {
	path := filepath.Join(
		os.Getenv("GOPATH"),
		"src",
		"github.com",
		"gonutz",
		"ld36",
		"rsc",
		filename,
	)
	return ioutil.ReadFile(path)
}

func readFileFromBlob(id string) (data []byte, err error) {
	var exists bool
	data, exists = rscBlob.GetByID(id)
	if !exists {
		err = errors.New("resource '" + id + "' does not exist in blob")
	}
	return
}

func mustLoadTexture(id string) (texture d3d9.Texture, width, height int) {
	nrgba := toNRGBA(mustLoadPng(id))
	width, height = nrgba.Bounds().Dx(), nrgba.Bounds().Dy()
	var err error
	texture, err = device.CreateTexture(
		uint(nrgba.Bounds().Dx()),
		uint(nrgba.Bounds().Dy()),
		1,
		d3d9.USAGE_SOFTWAREPROCESSING,
		d3d9.FMT_A8R8G8B8,
		d3d9.POOL_MANAGED,
		nil,
	)
	if err != nil {
		fatalf("unable to create texture %v: %v", id, err)
	}
	lockedRect, err := texture.LockRect(0, nil, d3d9.LOCK_DISCARD)
	if err != nil {
		fatalf("unable to lock texture %v: %v", id, err)
	}
	lockedRect.SetAllBytes(nrgba.Pix, nrgba.Stride)
	err = texture.UnlockRect(0)
	if err != nil {
		fatalf("unable to unlock texture %v: %v", id, err)
	}
	return
}

func mustLoadPng(id string) image.Image {
	data, err := readFile(id + ".png")
	if err != nil {
		fatalf("unable to load image %v.png: %v", id, err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		fatalf("image %v.png is not a valid png: %v", id, err)
	}
	return image
}

func toNRGBA(img image.Image) (nrgba *image.NRGBA) {
	if asNRGBA, ok := img.(*image.NRGBA); ok {
		nrgba = asNRGBA
	} else {
		nrgba = image.NewNRGBA(img.Bounds())
		draw.Draw(nrgba, nrgba.Bounds(), img, image.ZP, draw.Src)
	}
	return
}

func newGameResources() *resources {
	return &resources{
		images: make(map[string]game.Image),
	}
}

type resources struct {
	textures []d3d9.Texture
	images   map[string]game.Image
}

func (r *resources) close() {
	for i := range r.textures {
		r.textures[i].Release()
	}
}

func (r *resources) LoadImage(id string) game.Image {
	if img, ok := r.images[id]; ok {
		return img
	}

	texture, w, h := mustLoadTexture(id)
	r.textures = append(r.textures, texture)
	r.images[id] = textureImage{
		texture: texture,
		width:   w,
		height:  h,
	}

	logf("loaded texture %v (size %vx%v)\n", id, w, h)

	return r.images[id]
}

type textureImage struct {
	texture       d3d9.Texture
	width, height int
}

func uint32ToFloat32(value uint32) float32 {
	return *(*float32)(unsafe.Pointer(&value))
}

func (img textureImage) DrawAt(x, y int) {
	img.draw(x, y, false, 0)
}

func (img textureImage) DrawAtFlipX(x, y int, flipX bool) {
	img.draw(x, y, flipX, 0)
}

func (img textureImage) DrawAtRotatedCW(x, y, degrees int) {
	img.draw(x, y, false, degrees)
}

func (img textureImage) draw(x, y int, flipX bool, degrees int) {
	if err := device.SetTexture(0, img.texture.BaseTexture); err != nil {
		logln("DrawAt: device.SetTexture failed:", err)
		return
	}

	// the coordinate system for drawing goes from bottom to top
	y = windowH - 1 - img.height - y

	fx, fy := float32(x), float32(y)
	fw, fh := float32(img.width), float32(img.height)

	x1, y1 := -fw/2, -fh/2
	x2, y2 := fw/2, -fh/2
	x3, y3 := -fw/2, fh/2
	x4, y4 := fw/2, fh/2

	if flipX {
		x1, x2, x3, x4 = x2, x1, x4, x3
	}

	if degrees != 0 {
		s, c := math.Sincos(float64(degrees) / 180 * math.Pi)
		sin, cos := float32(s), float32(c)
		x1, y1 = cos*x1-sin*y1, sin*x1+cos*y1
		x2, y2 = cos*x2-sin*y2, sin*x2+cos*y2
		x3, y3 = cos*x3-sin*y3, sin*x3+cos*y3
		x4, y4 = cos*x4-sin*y4, sin*x4+cos*y4
	}

	dx := fx + fw/2 - 0.5
	dy := fy + fh/2 - 0.5
	white := uint32ToFloat32(0xFFFFFFFF)
	data := [...]float32{
		x1 + dx, y1 + dy, 0, 1, white, 0, 0,
		x2 + dx, y2 + dy, 0, 1, white, 1, 0,
		x3 + dx, y3 + dy, 0, 1, white, 0, 1,
		x4 + dx, y4 + dy, 0, 1, white, 1, 1,
	}
	if err := device.DrawPrimitiveUP(
		d3d9.PT_TRIANGLESTRIP,
		2,
		unsafe.Pointer(&data[0]),
		vertexStride,
	); err != nil {
		logln("DrawAt: device.DrawPrimitiveUP failed:", err)
	}

	// TODO reset the texture if necessary (if later allowing operations that
	// do not use textures)
	//if err := w.device.SetTexture(0, d3d9.BaseTexture{}); err != nil {
	//logln("DrawAt: device.SetTexture failed on reset:", err)
	//return
	//}
}

func (img textureImage) Size() (int, int) {
	return img.width, img.height
}

func log(a ...interface{})                 { logToFile(fmt.Sprint(a...)) }
func logf(format string, a ...interface{}) { logToFile(fmt.Sprintf(format, a...)) }
func logln(a ...interface{})               { logToFile(fmt.Sprintln(a...)) }

func logToFile(msg string) {
	if logFile == nil {
		path := filepath.Join(os.Getenv("APPDATA"), "ld36_log.txt")
		logFile, _ = os.Create(path)
	}

	fmt.Print(msg)

	if logFile != nil {
		logFile.Write([]byte(msg))
	}
}

func fatal(a ...interface{}) {
	msg := fmt.Sprint(a...)
	fail(msg)
}

func fatalf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fail(msg)
}

func fail(msg string) {
	logln("fatal error:", msg)
	panic(msg)
}
