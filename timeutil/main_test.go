package timeutil

import (
	"testing"
	"time"
)

func TestCalendar(t *testing.T) {
	bd := &DateTraveler{
		BusinessDays: true,
	}

	tm := time.Date(2020, 11, 13, 2, 3, 4, 5, time.UTC)
	tm = bd.AddDays(tm, 1)

	if tm.Weekday() != time.Monday {
		t.Fatal("should have skipped the weekend to monday")
	}

	y, m, d := tm.Date()
	if y != 2020 || m != 11 || d != 16 {
		t.Fatal("should be nov 16th", tm.String())
	}

	if tm.Hour() != 2 {
		t.Fatal("should have preserved hours")
	}
}

func TestCalendar2(t *testing.T) {
	bd := &DateTraveler{
		BusinessDays: true,
	}

	tm := time.Date(2020, 11, 10, 2, 3, 4, 5, time.UTC)
	tm = bd.AddDays(tm, 2)

	y, m, d := tm.Date()
	if y != 2020 || m != 11 || d != 12 {
		t.Fatal("should be nov 12th", tm.String())
	}

	if tm.Hour() != 2 {
		t.Fatal("should have preserved hours")
	}
}

func TestCalendar3(t *testing.T) {
	bd := &DateTraveler{
		BusinessDays: true,
		Holidays: []Date{
			{Y: 2020, M: 11, D: 11},
		},
	}

	tm := time.Date(2020, 11, 10, 2, 3, 4, 5, time.UTC)
	tm = bd.AddDays(tm, 2)

	y, m, d := tm.Date()
	if y != 2020 || m != 11 || d != 13 {
		t.Fatal("should be nov 13th", tm.String())
	}

	if tm.Hour() != 2 {
		t.Fatal("should have preserved hours")
	}
}

func TestNextWeekday(t *testing.T) {
	for i := time.Sunday; i <= time.Saturday; i++ {
		src := time.Date(2000, 01, 01+int(i), 0, 0, 0, 0, time.UTC)
		tm := NextWeekday(src, time.Wednesday)
		if tm.Weekday() != time.Wednesday {
			t.Error("invalid weekday for next wednesday after", src.String(), "got", tm.Weekday())
		}
	}
}

func TestSetTime(t *testing.T) {
	src := time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)
	dst := SetTime(src, 1, 59, 3)

	hr, min, sec := dst.Clock()
	if hr != 1 {
		t.Error("invalid hour", hr)
	}

	if min != 59 {
		t.Error("invalid min", min)
	}

	if sec != 3 {
		t.Error("invalid sec", sec)
	}
}
