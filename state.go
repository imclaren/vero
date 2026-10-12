package vero

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// State is everything the frontend draws, and the lock that guards it.
//
// Create one with Worker.NewState.  The worker emits it whenever it changes
// and a handler registered with Worker.Update replies with it, so a frontend
// never polls and never draws the state from before its own change.
//
// The worker reads it as JSON, under the same lock that Do takes, so there is
// no copy to get wrong: a snapshot cannot be encoded halfway through a change,
// and nothing can hand out a value that keeps changing underneath the
// comparison that decides whether to emit.
type State[T any] struct {
	mu sync.Mutex
	v  T
	w  *Worker
}

// Do runs f with the state, under the lock.  Every change goes through it.
//
//	jobs.Do(func(s *Status) { s.Jobs = append(s.Jobs, Job{}) })
//
// Do not keep the pointer, start a goroutine with it, or block in f: the
// worker cannot encode the state while f holds the lock.
func (s *State[T]) Do(f func(*T)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.v)
}

// JSON is the state, encoded under the lock.
//
// This is the only way the state leaves: a Go value handed out would share the
// slices and maps inside it, and whatever received it would be reading them
// while Do writes.  Use it to reply with the state by hand - Update does it
// for you - and pass the result straight back as the reply.
func (s *State[T]) JSON() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(s.v)
}

// encode is how the worker reads the state.
func (s *State[T]) encode() ([]byte, error) { return s.JSON() }

// Snapshot is a copy of the state as it is now, which nothing else
// changes, made from its JSON under the state's lock: for a handler that
// replies with the state after work of its own, so that vero gen types its
// reply as the state.
func (s *State[T]) Snapshot() (T, error) {
	var v T
	data, err := s.JSON()
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(data, &v)
	return v, err
}

// state is what a Worker keeps: a State[T] with its type parameter forgotten.
type state interface {
	encode() ([]byte, error)
}

// NewState gives this worker its state, and returns it for the application to
// change with Do.
//
//	jobs := vero.NewState(w, Status{Jobs: []Job{{ID: 1, Name: "Photos"}}})
//	jobs.Do(func(s *Status) { s.Jobs[0].Progress++ })
//
// One worker has one state: calling this twice replaces the first, because the
// event channel carries no name and a frontend would have no way to tell two
// kinds of event apart.
func NewState[T any](w *Worker, initial T) *State[T] {
	s := &State[T]{v: initial, w: w}
	w.state = s
	return s
}

// Update answers a request by changing the state, and replies with it.
//
//	jobs.Update("restartJob", func(s *Status, req RestartJob) error {
//	    s.Jobs[req.ID].Progress = 0
//	    return nil
//	})
//
// fn runs under the lock, with the state and the decoded request, and the
// reply is the state as it leaves.  So a frontend cannot draw the state from
// before its own change, and there is nothing to copy or forget to return.
//
// Returning an error sends that to the frontend and changes nothing else.
//
// The lock is held for the duration, so this is for changing state, not for
// work: anything slow belongs in a goroutine the handler starts, writing back
// through Do.  A handler that replies with something other than the state is
// Worker.Handle.
func UpdateWith[T any, Req any](s *State[T], name string, fn func(state *T, request Req) error) {
	Handle(s.w, name, func(_ context.Context, request Req) (json.RawMessage, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := fn(&s.v, request); err != nil {
			return nil, err
		}
		return json.Marshal(s.v)
	})
}

// ID is a request that carries nothing but an identifier, which is most of
// them: restart this, cancel that, open the other.  T is whatever your ids
// are.
//
//	jobs.Update("restartJob", func(s *Status, req vero.ID[int]) error {
//	    …
//	})
type ID[T any] struct {
	ID T `json:"id"`
}

// Every runs fn with the state, under the lock, every interval, until the
// frontend goes away.  It returns straight away: the loop is a goroutine of
// its own.
//
//	jobs.Every(200*time.Millisecond, func(s *Status) {
//	    for i := range s.Jobs { s.Jobs[i].Progress++ }
//	})
//
// This is where a worker's own work goes when it is periodic.  Writing the
// loop by hand costs a goroutine, a ticker that time.Tick never reclaims, and
// a nested Do.
func (s *State[T]) Every(interval time.Duration, fn func(*T)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.w.ctx.Done():
				return
			case <-ticker.C:
			}
			s.Do(fn)
		}
	}()
}

// Keyed is anything the frontend can name: a job with an id, a row with a
// primary key.  Key is what the ID in a request matches.
//
//	func (j Job) Key() int { return j.ID }
type Keyed[K comparable] interface {
	Key() K
}

// find returns the item the frontend means, or an error to show.
func find[T Keyed[K], K comparable](items []T, key K) (*T, error) {
	for i := range items {
		if items[i].Key() == key {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("no item with id %v", key)
}

// Edit finds the item the frontend means and changes it, which is most of
// what a handler does.
//
//	func (s *Status) Pause(req vero.ID[int]) error {
//	    return vero.Edit(s.Jobs, req.ID, func(j *Job) { j.Paused = !j.Paused })
//	}
//
// A request naming something that is gone is refused, and the worker carries
// on.
func Edit[T Keyed[K], K comparable](items []T, key K, change func(*T)) error {
	item, err := find(items, key)
	if err != nil {
		return err
	}
	change(item)
	return nil
}

// UpdateItem answers a request that names one item and changes it, which is
// what a button on a row sends.
//
//	func (j *Job) Pause() error { j.Paused = !j.Paused; return nil }
//
//	vero.UpdateItem(jobs, "pauseJob", (*Job).Pause)
//
// The request is an ID, the item is found by Key, change runs under the lock
// with a pointer into the state, and the reply is the new state.  A request
// naming something that is gone is refused and the worker carries on.
//
// Which collection is worked out from the item type: the state's one []J
// field, or the state itself when it is a []J.  A state with two fields of the
// same item type cannot say which, and says so at registration rather than
// guessing.
func UpdateItem[T any, J Keyed[K], K comparable](s *State[T], name string, change func(*J) error) {
	field := itemsField[T, J]()
	UpdateWith(s, name, func(state *T, req ID[K]) error {
		item, err := find(field(state), req.ID)
		if err != nil {
			return err
		}
		return change(item)
	})
}

// itemsField finds the []J in a T, once, at registration.
func itemsField[T any, J any]() func(*T) []J {
	t := reflect.TypeFor[T]()
	want := reflect.TypeFor[[]J]()

	if t == want {
		return func(state *T) []J { return *any(state).(*[]J) }
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("vero: %s is not a []%s and has no fields to hold one", t, reflect.TypeFor[J]()))
	}

	found := -1
	for i := range t.NumField() {
		if t.Field(i).Type == want {
			if found >= 0 {
				panic(fmt.Sprintf("vero: %s has more than one []%s, so a request cannot say which", t, reflect.TypeFor[J]()))
			}
			found = i
		}
	}
	if found < 0 {
		panic(fmt.Sprintf("vero: %s has no []%s", t, reflect.TypeFor[J]()))
	}
	return func(state *T) []J {
		return reflect.ValueOf(state).Elem().Field(found).Interface().([]J)
	}
}

// Update answers a request that carries nothing - a button with no payload -
// changes the state, and replies with it.
//
//	func (s *Status) Add() error { … }
//
//	vero.Update(jobs, "addJob", (*Status).Add)
//
// UpdateWith is the same for a request that carries something, and UpdateItem
// for one that names an item.
func Update[T any](s *State[T], name string, fn func(*T) error) {
	UpdateWith(s, name, func(state *T, _ struct{}) error { return fn(state) })
}
