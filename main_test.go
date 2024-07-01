package main

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func setupTestDB(t *testing.T) (*ClockApp, func()) {
	tempDir, err := os.MkdirTemp("", "clock_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	dbPath := tempDir + "/test.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Error opening database: %v", err)
	}
	err = ensureTable(db)
	if err != nil {
		t.Fatalf("Error creating table: %v", err)
	}
	app := &ClockApp{DB: db, Path: dbPath}
	return app, func() {
		db.Close()
		os.RemoveAll(tempDir)
	}
}

func TestClockInOut(t *testing.T) {
	app, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name     string
		action   clockAction
		category string
		wantErr  bool
	}{
		{"ClockIn", clockInAction, "", false},
		{"ClockInAgain", clockInAction, "", true},
		{"ClockOut", clockOutAction, "", false},
		{"ClockOutAgain", clockOutAction, "", true},
		{"ClockInDifferentCategory", clockInAction, "work", false},
		{"ClockOutDifferentCategory", clockOutAction, "personal", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := app.clockInOut(tt.action, tt.category)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReadRows(t *testing.T) {
	app, cleanup := setupTestDB(t)
	defer cleanup()

	// Add some test records
	assert.NoError(t, app.writeRow(clockInAction, "work"))
	time.Sleep(time.Second)
	assert.NoError(t, app.writeRow(clockOutAction, "work"))

	records, err := app.readRows(2)
	assert.NoError(t, err)
	assert.Len(t, records, 2)
	assert.Equal(t, clockOutAction, records[0].action)
	assert.Equal(t, clockInAction, records[1].action)
	assert.Equal(t, "work", records[0].category)
	assert.Equal(t, "work", records[1].category)

	// Check time difference
	startTime, _ := time.Parse("2006-01-02 15:04:05", records[1].time)
	endTime, _ := time.Parse("2006-01-02 15:04:05", records[0].time)
	assert.GreaterOrEqual(t, endTime.Sub(startTime), time.Second)
}

func TestParseCategory(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"NoArgs", []string{}, ""},
		{"OneArg", []string{"work"}, "work"},
		{"MultipleArgs", []string{"work", "extra"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCategory(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClockInOutErrors(t *testing.T) {
	tests := []struct {
		name    string
		actions []struct {
			action   clockAction
			category string
		}
		expectedError string
	}{
		{
			name: "ClockInTwice",
			actions: []struct {
				action   clockAction
				category string
			}{
				{clockInAction, "work"},
				{clockInAction, "work"},
			},
			expectedError: "already clocked in",
		},
		{
			name: "ClockOutTwice",
			actions: []struct {
				action   clockAction
				category string
			}{
				{clockInAction, "work"},
				{clockOutAction, "work"},
				{clockOutAction, "work"},
			},
			expectedError: "already clocked out",
		},
		{
			name: "ClockOutDifferentCategory",
			actions: []struct {
				action   clockAction
				category string
			}{
				{clockInAction, "work"},
				{clockOutAction, "personal"},
			},
			expectedError: "cannot clock out of a different category",
		},
		{
			name: "ClockOutWithoutClockIn",
			actions: []struct {
				action   clockAction
				category string
			}{
				{clockOutAction, "work"},
			},
			expectedError: "cannot clock out without clocking in first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, cleanup := setupTestDB(t)
			defer cleanup()
			for i, action := range tt.actions {
				err := app.clockInOut(action.action, action.category)
				if i == len(tt.actions)-1 {
					pass := assert.Error(t, err)
					if pass {
						assert.Contains(t, err.Error(), tt.expectedError)
					} else {
						t.Logf("Error: %v", err)
					}
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}
