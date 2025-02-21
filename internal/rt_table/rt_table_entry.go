package rt_table

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type RTTablesEntry struct {
	ID   int64
	Name string
}

func CreateRTTablesEntry(rt_tables_entry *RTTablesEntry) {

	const rtTablesFile = "/etc/iproute2/rt_tables"

	exists, err := TableExists(rtTablesFile, rt_tables_entry)

	if err != nil {
		fmt.Printf("Error checking rt_tables: %v\n", err)
		return
	}

	if exists {
		fmt.Println("Routing table entry already exists.")
		return
	}

	file, err := os.OpenFile(rtTablesFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening rt_tables: %v\n", err)
		return
	}

	defer file.Close()

	entry := fmt.Sprintf("%d %s\n", rt_tables_entry.ID, rt_tables_entry.Name)
	_, err = file.WriteString(entry)
	if err != nil {
		fmt.Printf("Error writing to rt_tables: %v\n", err)
		return
	}

	fmt.Println("Successfully added routing table entry:", entry)
}

func TableExists(filename string, rt_tables_entry *RTTablesEntry) (bool, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	content := string(data)
	entry := strconv.FormatInt(rt_tables_entry.ID, 10) + " " + string(rt_tables_entry.Name)

	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return true, nil
		}
	}
	return false, nil

}
