package main

//#include <Windows.h>
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/AllenDang/w32"
	"github.com/gonutz/blob"
	"github.com/gonutz/d3d9"
	"github.com/gonutz/mixer"
	"github.com/gonutz/payload"
)

func init() {
	runtime.LockOSThread()
}

const (
	version = "1"
)

var (
	readFile          func(id string) ([]byte, error) = readFileFromDisk
	rscBlob           *blob.Blob
	logFile           io.WriteCloser
	muted             bool
	previousPlacement C.WINDOWPLACEMENT
)

func main() {
	// close the log file at the end of the program
	defer func() {
		if logFile != nil {
			logFile.Close()
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
	windowW := uint(client.Right - client.Left)
	windowH := uint(client.Bottom - client.Top)

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
		maxScreenW, maxScreenH = windowW, windowH
	}

	device, _, err := d3d.CreateDevice(
		d3d9.ADAPTER_DEFAULT,
		d3d9.DEVTYPE_HAL,
		unsafe.Pointer(cWindow),
		d3d9.CREATE_HARDWARE_VERTEXPROCESSING,
		d3d9.PRESENT_PARAMETERS{
			BackBufferWidth:  maxScreenW,
			BackBufferHeight: maxScreenH,
			BackBufferFormat: d3d9.FMT_A8R8G8B8,
			BackBufferCount:  1,
			Windowed:         true,
			SwapEffect:       d3d9.SWAPEFFECT_DISCARD,
			HDeviceWindow:    unsafe.Pointer(cWindow),
		},
	)
	if err != nil {
		fatal("unable to create Direct3D09 device: ", err)
	}
	defer device.Release()

	device.SetRenderState(d3d9.RS_CULLMODE, uint32(d3d9.CULL_CW))
	device.SetRenderState(d3d9.RS_SRCBLEND, d3d9.BLEND_SRCALPHA)
	device.SetRenderState(d3d9.RS_DESTBLEND, d3d9.BLEND_INVSRCALPHA)
	device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 1)

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
			// TODO render game
			// TODO check device lost error
			device.Present(
				&d3d9.RECT{0, 0, int32(windowW), int32(windowH)},
				nil,
				nil,
				nil,
			)
		}
	}
}

func handleMessage(window w32.HWND, message uint32, w, l uintptr) uintptr {
	switch message {
	case w32.WM_KEYDOWN:
		switch w {
		case w32.VK_F11:
			toggleFullscreen((C.HWND)(unsafe.Pointer(window)))
		}
		return 1
	case w32.WM_DESTROY:
		w32.PostQuitMessage(0)
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

func readFileFromDisk(id string) ([]byte, error) {
	path := "./rsc" + id
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
	const MB_TOPMOST = 0x00040000
	w32.MessageBox(0, msg, "Error", w32.MB_OK|w32.MB_ICONERROR|MB_TOPMOST)
	log("fatal error: ", msg)
	panic(msg)
}
