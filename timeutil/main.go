package timeutil

import (
	"github.com/ntbosscher/gobase/randomish"
	"log"
	"time"
)

// DateTraveler is a date-based time util that applies certain filters
// to which days are used
type DateTraveler struct {
	Holidays     []Date
	BusinessDays bool
}

// AddDays adds n days to the date skipping holidays and business days (if enabled)
// *note: time of day is preserved
func (c *DateTraveler) AddDays(when time.Time, n int) time.Time {
	for i := 0; i < n; {
		when = when.AddDate(0, 0, 1)
		if c.Contains(when) {
			i++
		}
	}

	return when
}

func (c *DateTraveler) Contains(when time.Time) bool {

	if c.BusinessDays {
		switch when.Weekday() {
		case time.Sunday:
			return false
		case time.Saturday:
			return false
		}
	}

	y, m, d := when.Date()

	for _, item := range c.Holidays {
		if item.Equals(y,m,d) {
			return false
		}
	}

	return true
}

type Date struct {
	Y int
	M time.Month
	D int
}

func (dt *Date) Equals(y int, m time.Month, d int) bool {
	return y == dt.Y && m == dt.M && d == dt.D
}

func AddBusinessDays(when time.Time, n int, holidays ...Date) time.Time {
	dt := &DateTraveler{
		Holidays:     holidays,
		BusinessDays: true,
	}

	return dt.AddDays(when, n)
}

// ScheduleJob runs the callback on the interval given
// Panics within the callback are logged but do not stop the interval processor
func ScheduleJob(interval time.Duration, callback func()) {

	wrappedCallback := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println(r)
			}
		}()

		callback()
	}

	go func() {
		// delay initial call somewhat randomly to ensure
		// we don't block startup and callbacks with the same interval don't run at exactly the same time
		<-time.After(interval / time.Duration(randomish.Int(1, 5)))

		tc := time.NewTicker(interval)
		defer tc.Stop()

		for range tc.C {
			wrappedCallback()
		}
	}()
}