//go:build windows

package main

import (
	"log"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Windows n'autorise une fenêtre à passer au premier plan que si son programme
// a été lancé par l'utilisateur. Ici c'est le lanceur qui ouvre Chrome : sans
// précaution, la fenêtre de l'application s'ouvre derrière les autres.

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	pAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	pEnumWindows              = user32.NewProc("EnumWindows")
	pGetClassNameW            = user32.NewProc("GetClassNameW")
	pGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pIsIconic                 = user32.NewProc("IsIconic")
	pShowWindow               = user32.NewProc("ShowWindow")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pBringWindowToTop         = user32.NewProc("BringWindowToTop")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pAttachThreadInput        = user32.NewProc("AttachThreadInput")
	pKeybdEvent               = user32.NewProc("keybd_event")
	pGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	pFlashWindowEx            = user32.NewProc("FlashWindowEx")
	pGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	rappelFenetres            = syscall.NewCallback(surFenetre)
	fenetresVues              map[uintptr]bool
)

// fenetresChrome : fenêtres principales visibles de Chrome / Edge.
func fenetresChrome() map[uintptr]bool {
	fenetresVues = map[uintptr]bool{}
	pEnumWindows.Call(rappelFenetres, 0)
	return fenetresVues
}

func surFenetre(h, _ uintptr) uintptr {
	if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
		return 1
	}
	buf := make([]uint16, 64)
	pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if syscall.UTF16ToString(buf) != "Chrome_WidgetWin_1" {
		return 1
	}
	if n, _, _ := pGetWindowTextLengthW.Call(h); n == 0 {
		return 1 // fenêtres techniques sans titre
	}
	const gwlExStyle, wsExToolWindow = ^uintptr(19), 0x80 // GWL_EXSTYLE = -20
	if st, _, _ := pGetWindowLongPtrW.Call(h, gwlExStyle); st&wsExToolWindow != 0 {
		return 1
	}
	fenetresVues[h] = true
	return 1
}

// preparerPremierPlan est appelée juste avant d'ouvrir le navigateur. Elle
// cède le droit de premier plan et renvoie une fonction qui, lancée ensuite,
// ramène devant la nouvelle fenêtre dès qu'elle apparaît.
func preparerPremierPlan() func() {
	pAllowSetForegroundWindow.Call(uintptr(0xFFFFFFFF)) // ASFW_ANY
	avant := fenetresChrome()
	attentePremierPlan.Add(1)
	return func() {
		defer attentePremierPlan.Done()
		runtime.LockOSThread() // AttachThreadInput exige un fil système fixe
		defer runtime.UnlockOSThread()
		fin := time.Now().Add(30 * time.Second)
		for time.Now().Before(fin) {
			time.Sleep(200 * time.Millisecond)
			for h := range fenetresChrome() {
				if avant[h] {
					continue
				}
				time.Sleep(300 * time.Millisecond) // laisser la fenêtre finir de s'afficher
				amenerDevant(h)
				return
			}
		}
		log.Printf("premier plan : aucune nouvelle fenêtre repérée")
	}
}

func amenerDevant(h uintptr) {
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		pShowWindow.Call(h, 9) // SW_RESTORE
	}
	pSetForegroundWindow.Call(h)
	if f, _, _ := pGetForegroundWindow.Call(); f == h {
		log.Printf("premier plan : fenêtre amenée devant")
		return
	}
	// Refusé : on rattache momentanément notre fil à celui de la fenêtre active
	// et on simule l'appui sur Alt, ce qui lève le verrou de Windows.
	f, _, _ := pGetForegroundWindow.Call()
	filActif, _, _ := pGetWindowThreadProcessId.Call(f, 0)
	monFil, _, _ := pGetCurrentThreadId.Call()
	if filActif != 0 && filActif != monFil {
		pAttachThreadInput.Call(monFil, filActif, 1)
		defer pAttachThreadInput.Call(monFil, filActif, 0)
	}
	pKeybdEvent.Call(0x12, 0, 0, 0) // Alt enfoncé
	pKeybdEvent.Call(0x12, 0, 2, 0) // Alt relâché
	pBringWindowToTop.Call(h)
	pSetForegroundWindow.Call(h)
	if f, _, _ := pGetForegroundWindow.Call(); f == h {
		log.Printf("premier plan : fenêtre amenée devant (2e essai)")
		return
	}
	// Dernier recours : le bouton de la barre des tâches clignote.
	type flashInfo struct {
		taille                  uint32
		fenetre                 uintptr
		drapeaux, nombre, delai uint32
	}
	fi := flashInfo{fenetre: h, drapeaux: 3 | 12, nombre: 3} // FLASHW_ALL | FLASHW_TIMERNOFG
	fi.taille = uint32(unsafe.Sizeof(fi))
	pFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
	log.Printf("premier plan : refusé par Windows, bouton de la barre des tâches mis en évidence")
}
