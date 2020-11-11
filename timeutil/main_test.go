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
