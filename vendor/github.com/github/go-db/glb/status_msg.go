package glb

import (
	"encoding/csv"
	"io"
	"strings"
)

// StatusMessage is the output from hitting the GLB status port
type StatusMessage struct {
	Columns map[string]int
	Data    [][]string
}

// FieldAt returns the given colum name for the row number
func (m *StatusMessage) FieldAt(colname string, rownum int) string {
	if col, ok := m.Columns[colname]; ok {
		return m.Data[rownum][col]
	}
	return ""
}

// Rows returns the number of data rows
func (m *StatusMessage) Rows() int {
	return len(m.Data)
}

// NewStatusMessage parses the CSV output as raw text from the reader object
func NewStatusMessage(r io.Reader) (*StatusMessage, error) {
	csvData := csv.NewReader(r)
	record, err := csvData.Read()
	if err != nil {
		return nil, err
	}

	msg := StatusMessage{
		Columns: map[string]int{},
	}

	// Create a reverse lookup of column names to numbers, it's assumed that
	// the first row is a header here
	for i, val := range record {
		// First line starts with a # character we don't want as part of the column name!
		val = strings.Replace(val, "# ", "", -1)
		msg.Columns[val] = i
	}

	// Then load the rest of the lines as data
	msg.Data, err = csvData.ReadAll()
	if err != nil {
		return nil, err
	}

	return &msg, nil
}
