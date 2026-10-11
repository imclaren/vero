package main

import "testing"

// TestPlanMacNeedsProfile checks that the Mac is recorded only when [test]
// gives the app a profile, so that a recording never uses the settings of
// the copy in use.
func TestPlanMacNeedsProfile(t *testing.T) {
	mac := func(p Plan) (PlanStep, bool) {
		for _, s := range p.Test {
			if s.Name == "macos" {
				return s, true
			}
		}
		return PlanStep{}, false
	}
	a := &App{Name: "counter", MacOS: &MacOS{}}
	if _, ok := mac(planFor(a)); !ok {
		t.Skip("the Mac app cannot be built here")
	}
	for _, c := range []struct {
		test *Test
		ok   bool
	}{
		{nil, false},
		{&Test{Env: map[string]string{"OTHER": "1"}}, false},
		{&Test{Env: map[string]string{"VERO_PROFILE": "demo"}}, true},
	} {
		a.Test = c.test
		s, _ := mac(planFor(a))
		if s.OK != c.ok || (!s.OK && s.Why == "") {
			t.Errorf("with [test] %+v, the Mac's step is %+v", c.test, s)
		}
	}
}
