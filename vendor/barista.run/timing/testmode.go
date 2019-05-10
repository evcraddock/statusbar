// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package timing

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"barista.run/base/watchers/localtz"
	l "barista.run/logging"
)

type trigger struct {
	what *Scheduler
	when time.Time
}

type triggerList []trigger

func (l triggerList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l triggerList) Len() int           { return len(l) }
func (l triggerList) Less(i, j int) bool { return l[i].when.Before(l[j].when) }

var (
	triggers   triggerList
	triggersMu sync.Mutex
)

// nowInTest tracks the current time in test mode.
var nowInTest atomic.Value // of time.Time

// testModeID tracks the test instance, to prevent test schedulers from crossing
// test boundaries. Each call to TestMode() changes this ID, and any schedulers
// with a different ID are ignored.
var testModeID uint32

func testNow() time.Time {
	return nowInTest.Load().(time.Time)
}

// TestMode sets test mode for all schedulers.
// In test mode schedulers do not fire automatically, and time
// does not pass at all, until NextTick() or Advance* is called.
func TestMode() {
	reset(func() {
		testMode = true
		testModeID++
		// Set to non-zero time when entering test mode so that any IsZero
		// checks don't unexpectedly pass.
		nowInTest.Store(time.Date(2016, time.November, 25, 20, 47, 0, 0, time.UTC))
		// Also simplify tests by fixing the timezone.
		localtz.SetForTest(time.UTC)
	})
}

// ExitTestMode exits test mode for all schedulers. Any schedulers created
// after this call will be real.
func ExitTestMode() {
	reset(func() {
		testMode = false
		localtz.SetForTest(time.Local)
	})
}

func reset(fn func()) {
	mu.Lock()
	defer mu.Unlock()
	triggersMu.Lock()
	defer triggersMu.Unlock()
	fn()
	waiters = nil
	triggers = nil
	paused = false
}

func (s *Scheduler) setNextTrigger(when time.Time) *Scheduler {
	newTriggers := triggerList{}
	triggersMu.Lock()
	defer triggersMu.Unlock()
	for _, t := range triggers {
		if t.what != s && t.what.testModeID == testModeID {
			newTriggers = append(newTriggers, t)
		}
	}
	triggers = newTriggers
	if !when.IsZero() && s.testModeID == testModeID {
		triggers = append(triggers, trigger{s, when})
	}
	sort.Sort(triggers)
	return s
}

func (s *Scheduler) nextRepeatingTick() time.Time {
	elapsedIntervals := Now().Sub(s.startTime) / s.interval
	return s.startTime.Add(s.interval * (elapsedIntervals + 1))
}

func (s *Scheduler) testModeAt(when time.Time) *Scheduler {
	l.Fine("%s At[Test](%v)", l.ID(s), when)
	return s.setNextTrigger(when)
}

func (s *Scheduler) testModeAfter(delay time.Duration) *Scheduler {
	l.Fine("%s After[Test](%v)", l.ID(s), delay)
	return s.setNextTrigger(Now().Add(delay))
}

func (s *Scheduler) testModeEvery(interval time.Duration) *Scheduler {
	l.Fine("%s Every[Test](%v)", l.ID(s), interval)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startTime = Now()
	s.interval = interval
	return s.setNextTrigger(s.nextRepeatingTick())
}

func (s *Scheduler) testModeStop() {
	l.Fine("%s Stop[Test]", l.ID(s))
	s.setNextTrigger(time.Time{})
}

// NextTick triggers the next scheduler and returns the trigger time.
// It also advances test time to match.
func NextTick() time.Time {
	triggersMu.Lock()
	defer triggersMu.Unlock()
	if len(triggers) == 0 {
		return testNow()
	}
	when := triggers[0].when
	return advanceToLocked(when)
}

// AdvanceBy increments the test time by the given duration,
// and triggers any schedulers that were scheduled in the meantime.
func AdvanceBy(duration time.Duration) time.Time {
	return AdvanceTo(Now().Add(duration))
}

// AdvanceTo increments the test time to the given time,
// and triggers any schedulers that were scheduled in the meantime.
func AdvanceTo(newTime time.Time) time.Time {
	triggersMu.Lock()
	defer triggersMu.Unlock()
	return advanceToLocked(newTime)
}

func advanceToLocked(newTime time.Time) time.Time {
	if len(triggers) == 0 {
		nowInTest.Store(newTime)
		return newTime
	}
	nextTick := triggers[0].when
	if nextTick.After(newTime) {
		nowInTest.Store(newTime)
		return newTime
	}
	now := testNow()
	if nextTick.After(now) {
		nowInTest.Store(nextTick)
	} else {
		nextTick = now
	}
	idx := 0
	for i, t := range triggers {
		if triggers[i].when.After(nextTick) {
			break
		}
		if t.what.interval > 0 {
			t.when = t.what.nextRepeatingTick()
			triggers = append(triggers, t)
		}
		idx = i + 1
		t.what.maybeTrigger()
	}
	triggers = triggers[idx:]
	sort.Sort(triggers)
	if newTime.After(nextTick) {
		return advanceToLocked(newTime)
	}
	return nextTick
}
