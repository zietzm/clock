package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"slices"
	"text/tabwriter"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

type ClockApp struct {
	DB   *sql.DB
	Path string
}

func NewClockApp() (*ClockApp, error) {
	path, err := ensureDbPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}
	err = ensureTable(db)
	if err != nil {
		return nil, err
	}
	return &ClockApp{DB: db, Path: path}, nil
}

func ensureDbPath() (string, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting home directory: %v", err)
	}
	dir := homedir + "/.clock"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("error creating directory: %v", err)
		}
	}
	return homedir + "/.clock/clock.db", nil
}

func ensureTable(db *sql.DB) error {
	sqlStmt := `create table if not exists records 
    (id integer not null primary key, time text, action text, category text);`
	_, err := db.Exec(sqlStmt)
	if err != nil {
		return fmt.Errorf("error creating table: %v", err)
	}
	return nil
}

type clockAction string

const (
	clockInAction  clockAction = "in"
	clockOutAction clockAction = "out"
)

type Record struct {
	id       int
	time     string
	action   clockAction
	category string
}

func (app *ClockApp) readRows(n int) ([]Record, error) {
	rows, err := app.DB.Query(
		"select id, time, action, category from records order by id desc limit ?;",
		n,
	)
	if err != nil {
		return nil, fmt.Errorf("error getting last %d records: %v", n, err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var record Record
		err = rows.Scan(&record.id, &record.time, &record.action, &record.category)
		if err != nil {
			return nil, fmt.Errorf("error scanning record: %v", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (app *ClockApp) writeRow(action clockAction, category string) error {
	_, err := app.DB.Exec(
		"insert into records (time, action, category) values (datetime('now'), ?, ?);",
		action,
		category,
	)
	if err != nil {
		return fmt.Errorf("error inserting record: %v", err)
	}
	return nil
}

func (app *ClockApp) clockInOut(action clockAction, category string) error {
	states, err := app.readRows(1)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		switch action {
		case clockInAction:
			return app.writeRow(action, category)
		case clockOutAction:
			return fmt.Errorf("cannot clock out without clocking in first")
		}
	}
	state := states[0]
	if (action == clockInAction) && (state.action == clockInAction) {
		return fmt.Errorf("already clocked in (%s @ %v)", state.category, state.time)
	}
	if (action == clockOutAction) && (state.action == clockOutAction) {
		return fmt.Errorf("already clocked out (%s @ %v)", state.category, state.time)
	}
	if (action == clockOutAction) && (state.action == clockInAction) {
		if (category != "") && (state.category != category) {
			return fmt.Errorf("cannot clock out of a different category (%s)", state.category)
		}
		if category == "" {
			category = state.category
		}
	}
	return app.writeRow(action, category)
}

func (app *ClockApp) printTimeElapsed() error {
	records, err := app.readRows(2)
	if err != nil {
		return err
	}
	if len(records) < 2 {
		return fmt.Errorf("not enough records to calculate time elapsed")
	}
	startTime, err := time.Parse("2006-01-02 15:04:05", records[1].time)
	if err != nil {
		return fmt.Errorf("error parsing start time: %v", err)
	}
	endTime, err := time.Parse("2006-01-02 15:04:05", records[0].time)
	if err != nil {
		return fmt.Errorf("error parsing end time: %v", err)
	}
	elapsed := endTime.Sub(startTime)
	if records[0].action == clockInAction {
		fmt.Printf("Last clock in was from %v to %v (%v)\n", startTime, endTime, elapsed)
	} else {
		fmt.Printf("Last clock out was %v (%v ago)\n", endTime, elapsed)
	}
	return nil
}

func (app *ClockApp) printLog(n int) error {
	records, err := app.readRows(n)
	if err != nil {
		return err
	}
	slices.Reverse(records)
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tAction\tCategory\tTime")
	for _, record := range records {
		fmt.Fprintf(
			w,
			"%d:\t%s\t%s\t%s\n",
			record.id,
			record.action,
			record.category,
			record.time,
		)
	}
	w.Flush()
	return nil
}

func parseCategory(args []string) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		return args[0]
	default:
		return ""
	}
}

func main() {
	log.SetFlags(0)
	app, err := NewClockApp()
	if err != nil {
		log.Fatal(err)
	}

	var clockInCmd = &cobra.Command{
		Use:   "in",
		Short: "Clock in",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			category := parseCategory(args)
			err := app.clockInOut(clockInAction, category)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	var clockOutCmd = &cobra.Command{
		Use:   "out",
		Short: "Clock out",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			category := parseCategory(args)
			err := app.clockInOut(clockOutAction, category)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	var n int
	var clockLogCmd = &cobra.Command{
		Use:   "log",
		Short: "Show the log of recent clock actions",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			err := app.printLog(n)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	clockLogCmd.Flags().
		IntVarP(&n, "number of records to show", "n", 10, "Number of records to show")

	var clockStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show the current status",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			records, err := app.readRows(1)
			if err != nil {
				log.Fatal(err)
			}
			if len(records) == 0 {
				log.Fatalln("No records found")
			}
			record := records[0]
			startTime, err := time.Parse("2006-01-02 15:04:05", record.time)
			if err != nil {
				log.Fatal(err)
			}
			now := time.Now()
			elapsed := now.Sub(startTime).Round(time.Second)
			fmt.Printf(
				"Clock: %s @ %s from %s (%s)\n",
				record.action,
				record.category,
				record.time,
				elapsed,
			)
		},
	}

	var rootCmd = &cobra.Command{Use: "clock"}
	rootCmd.AddCommand(clockInCmd, clockOutCmd, clockLogCmd, clockStatusCmd)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.Execute()
}
