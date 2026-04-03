// Copyright 2013 The imageproxy authors.
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestParseStorages(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantCount int
		wantURL   string
		wantError bool
	}{
		{
			name:      "valid storages",
			value:     `{"tms":"https://tms.example.com/media/","hr":"https://hr.example.com/assets/"}`,
			wantCount: 2,
			wantURL:   "https://tms.example.com/media/",
		},
		{
			name:      "empty object",
			value:     `{}`,
			wantCount: 0,
		},
		{
			name:      "empty name",
			value:     `{"":"https://tms.example.com/media/"}`,
			wantError: true,
		},
		{
			name:      "slash in name",
			value:     `{"tm/s":"https://tms.example.com/media/"}`,
			wantError: true,
		},
		{
			name:      "relative url",
			value:     `{"tms":"/media/"}`,
			wantError: true,
		},
		{
			name:      "unsupported scheme",
			value:     `{"tms":"s3://bucket/media/"}`,
			wantError: true,
		},
		{
			name:      "invalid json",
			value:     `{"tms":`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStorages(tt.value)
			if tt.wantError {
				if err == nil {
					t.Fatalf("parseStorages(%q) did not return an error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStorages(%q) returned unexpected error: %v", tt.value, err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("parseStorages(%q) returned %d storages, want %d", tt.value, len(got), tt.wantCount)
			}
			if tt.wantURL != "" && got["tms"].String() != tt.wantURL {
				t.Errorf("parseStorages(%q) returned %q for tms, want %q", tt.value, got["tms"], tt.wantURL)
			}
		})
	}
}

func TestPrefixBaseURLsFlagSet(t *testing.T) {
	var storages prefixBaseURLsFlag

	if err := storages.Set(`{"tms":"https://tms.example.com/media/"}`); err != nil {
		t.Fatalf("first Set returned unexpected error: %v", err)
	}
	if err := storages.Set(`{"hr":"https://hr.example.com/assets/"}`); err != nil {
		t.Fatalf("second Set returned unexpected error: %v", err)
	}

	if len(storages) != 2 {
		t.Fatalf("Set should merge storages, got %d items", len(storages))
	}
	if got, want := storages["hr"].String(), "https://hr.example.com/assets/"; got != want {
		t.Errorf("storages[\"hr\"] = %q, want %q", got, want)
	}
}
