// Command plan9-app is the Plan 9 frontend: the same three rows as every
// other example, drawn with libdraw in a rio window.
//
//	scripts/run-plan9.sh --app
//
// Plan 9 has no toolkit, and draws through libdraw: a window is an image,
// and a program draws on it.  This uses a Go port of that library, which
// speaks to /dev/draw directly, so the frontend is Go on this platform too -
// and so it uses vero's own supervisor rather than a binding.  The worker
// is ../worker, built for plan9/amd64 and started as a process beside this
// one, exactly as it is on macOS, Windows and the BSDs.
//
// The library is pinned to an April 2021 revision in go.mod.  Later ones do
// not compile for GOOS=plan9: three lines of mux_plan9.go went stale, and
// nobody building for Plan 9 noticed, which says something about how often
// that happens.
package main

import (
	"context"
	"encoding/json"
	"image"
	"log"
	"os"
	"path/filepath"
	"time"

	"9fans.net/go/draw"
	"github.com/imclaren/vero"
)

// The shape the worker sends: the same Status and Job as ../worker.
type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

const (
	rowHeight = 32
	margin    = 16
	buttonW   = 72
	nameW     = 96
	phaseW    = 170
)

func main() {
	worker := "worker"
	if len(os.Args) > 1 {
		worker = os.Args[1]
	}
	if dir, err := os.Getwd(); err == nil && !filepath.IsAbs(worker) {
		worker = filepath.Join(dir, worker)
	}

	// The window: libdraw attaches to the rio window this was started in.
	d, err := draw.Init(nil, "", "vero", "480x160")
	if err != nil {
		log.Fatalf("draw: %v", err)
	}
	ui := newUI(d)

	events := make(chan Status, 16)
	sup := vero.Supervise(vero.SupervisorOptions{
		Path: worker,
		OnEvent: func(e json.RawMessage) {
			var s Status
			if json.Unmarshal(e, &s) == nil {
				select {
				case events <- s:
				default:
				}
			}
		},
		// No OnLog.  Standard error is the rio window this program draws
		// in, and the worker's log would print straight over the rows.
	})
	defer sup.Stop()
	if err := sup.Err(); err != nil {
		log.Fatalf("vero: %v", err)
	}

	mouse := d.InitMouse()
	kbd := d.InitKeyboard()
	ui.draw()

	for {
		select {
		case s := <-events:
			ui.status = s
			ui.draw()
		case m := <-mouse.C:
			if m.Buttons&1 != 0 {
				if id := ui.buttonAt(m.Point); id != 0 {
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						if reply, err := sup.Call(ctx, "restartJob", vero.ID[int]{ID: id}); err == nil {
							var s Status
							if json.Unmarshal(reply, &s) == nil {
								events <- s
							}
						}
					}()
				}
			}
		case <-mouse.Resize:
			if err := d.Attach(draw.RefNone); err == nil {
				ui.draw()
			}
		case r := <-kbd.C:
			if r == 'q' || r == 0x7f { // q, or Del
				return
			}
		}
	}
}

// ui draws the rows.  Everything is an image: the window, the colours, the
// text.  There is no widget to ask for a button, so a button is a rectangle
// with a word in it, and a click is a point inside that rectangle.
type ui struct {
	d                       *draw.Display
	status                  Status
	bg, fg, dim, bar, track *draw.Image
	buttons                 map[int]image.Rectangle
}

func newUI(d *draw.Display) *ui {
	solid := func(c draw.Color) *draw.Image {
		img, err := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, true, c)
		if err != nil {
			log.Fatalf("alloc: %v", err)
		}
		return img
	}
	return &ui{
		d:       d,
		bg:      solid(draw.White),
		fg:      solid(draw.Black),
		dim:     solid(0x777777FF),
		bar:     solid(0x3478F6FF),
		track:   solid(0xDDDDDDFF),
		buttons: map[int]image.Rectangle{},
	}
}

func (u *ui) draw() {
	d := u.d
	screen := d.ScreenImage
	font := d.Font
	screen.Draw(screen.R, u.bg, nil, draw.ZP)

	for i, job := range u.status.Jobs {
		top := screen.R.Min.Y + margin + i*(rowHeight+8)
		x := screen.R.Min.X + margin
		textY := top + (rowHeight-font.Height)/2

		// The button: a border, and "Restart" centred inside it.
		b := image.Rect(x, top, x+buttonW, top+rowHeight)
		screen.Border(b, 1, u.fg, draw.ZP)
		label := "Restart"
		lp := image.Pt(b.Min.X+(buttonW-font.StringWidth(label))/2, textY)
		screen.String(lp, u.fg, draw.ZP, font, label)
		u.buttons[job.ID] = b
		x += buttonW + 12

		screen.String(image.Pt(x, textY), u.fg, draw.ZP, font, job.Name)
		x += nameW
		screen.String(image.Pt(x, textY), u.dim, draw.ZP, font, job.Phase)
		x += phaseW

		// The progress bar: a track, and a fill proportional to progress.
		right := screen.R.Max.X - margin
		if right > x+20 {
			track := image.Rect(x, top+rowHeight/2-3, right, top+rowHeight/2+3)
			screen.Draw(track, u.track, nil, draw.ZP)
			fill := track
			fill.Max.X = track.Min.X + (track.Dx() * job.Progress / 100)
			screen.Draw(fill, u.bar, nil, draw.ZP)
		}
	}
	if len(u.status.Jobs) == 0 {
		screen.String(image.Pt(screen.R.Min.X+margin, screen.R.Min.Y+margin), u.dim, draw.ZP, font, "waiting for the worker")
	}
	d.Flush()
}

func (u *ui) buttonAt(p image.Point) int {
	for id, r := range u.buttons {
		if p.In(r) {
			return id
		}
	}
	return 0
}
