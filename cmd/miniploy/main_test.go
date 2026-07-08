package main

import (
	"testing"
	"time"
)

func TestJitteredIntervalRange(t *testing.T) {
	interval := 5 * time.Minute
	min := interval - interval/10
	max := interval + interval/10

	for range 100 {
		got := jitteredInterval(interval)
		if got < min || got > max {
			t.Fatalf("jitteredInterval(%v) = %v, want between %v and %v", interval, got, min, max)
		}
	}
}

func TestJitteredIntervalTinyInterval(t *testing.T) {
	interval := time.Nanosecond

	if got := jitteredInterval(interval); got != interval {
		t.Fatalf("jitteredInterval(%v) = %v, want %v", interval, got, interval)
	}
}
