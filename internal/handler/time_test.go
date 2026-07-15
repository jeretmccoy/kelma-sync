package handler

import (
	"testing"
	"time"
)

func TestUTCWriteTimesNormalizesOffsets(t *testing.T) {
	now := time.Date(2026, 7, 14, 18, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	client := time.Date(2026, 7, 15, 6, 30, 0, 0, time.FixedZone("JST", 9*60*60))

	normalizedNow, normalizedClient := utcWriteTimes(now, client)

	if got, want := normalizedNow.Format(time.RFC3339), "2026-07-14T22:00:00Z"; got != want {
		t.Fatalf("server time = %s, want %s", got, want)
	}
	if got, want := normalizedClient.Format(time.RFC3339), "2026-07-14T21:30:00Z"; got != want {
		t.Fatalf("client time = %s, want %s", got, want)
	}
}

func TestUTCWriteTimesClampsFutureAndMissingClientClocks(t *testing.T) {
	now := time.Date(2026, 7, 14, 22, 0, 0, 0, time.UTC)

	_, future := utcWriteTimes(now, now.Add(maxFutureClockSkew+time.Second))
	if !future.Equal(now) {
		t.Fatalf("future client time = %s, want %s", future, now)
	}

	_, missing := utcWriteTimes(now, time.Time{})
	if !missing.Equal(now) {
		t.Fatalf("missing client time = %s, want %s", missing, now)
	}
}
