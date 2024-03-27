package util

import (
	"api/shared/models"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

func AnalyzeOneDimensionalData(csvFile *os.File, columnConfig *models.ReportCSVData) error {
	records, err := readCSVFromHandler(csvFile)
	if err != nil {
		return err
	}
	output := make(map[string]interface{})
	headers := records[0]
	columnIndex := findColumnIndex(headers, columnConfig.OperationColumn)
	if columnIndex == -1 {
		return fmt.Errorf("column '%s' not found in the CSV file", columnConfig.OperationColumn)
	}
	transformedInput := models.ReportOneDimConfig{
		AggregateValueLabel: columnConfig.Label,
		Column:              columnConfig.OperationColumn,
		OperationType:       columnConfig.OperationType,
		FilterColumns:       columnConfig.FilterColumns,
		AcceptedValues:      columnConfig.AcceptedValues,
	}
	for _, record := range records[1:] {
		err := processOperation(record, output, &transformedInput, columnIndex, headers, true)
		if err != nil {
			return fmt.Errorf("error occurred during process operation: '%s", err)
		}
	}
	if columnConfig.OperationType == models.Average {
		calculateAverages(output, &transformedInput)
	}
	fmt.Println(output)
	value, ok := output[columnConfig.Label]
	// No counts were found. Set to zero
	if !ok {
		value = 0
	}
	switch v := value.(type) {
	case int:
		columnConfig.Result = strconv.Itoa(v)
	case float64:
		columnConfig.Result = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Errorf("value for key is neither an int nor a float64")
	}

	// Seek to the beginning of the file to allow for reading again
	_, err = csvFile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

func AnalyzeTwoDimensionalData(csvFile *os.File, reportOutput *models.ReportChartOutput) error {
	records, err := readCSVFromHandler(csvFile)
	if err != nil {
		return err
	}

	headers := records[0]
	independentColumnIndex := findColumnIndex(headers, reportOutput.IndependentColumn)
	if independentColumnIndex == -1 {
		return fmt.Errorf("column '%s' not found in the CSV file", reportOutput.IndependentColumn)
	}
	output := make(map[string]map[string]interface{})
	for _, record := range records[1:] {
		independentValue := record[independentColumnIndex]
		if !rowPassesFilters(record, reportOutput.FilterColumns, headers) || !rowPassesFilters(record,
			map[string][]string{
				reportOutput.IndependentColumn: reportOutput.AcceptedValues,
			}, headers) {
			continue
		}
		if _, exists := output[independentValue]; !exists {
			output[independentValue] = make(map[string]interface{})
		}
		for _, columnConfig := range reportOutput.DependentColumns {
			columnIndex := findColumnIndex(headers, columnConfig.Column)
			if columnIndex == -1 {
				return fmt.Errorf("column '%s' not found in the CSV file", columnConfig.Column)
			}
			err := processOperation(record, output[independentValue], &columnConfig, columnIndex, headers, false)
			if err != nil {
				return err
			}
		}
	}
	for _, data := range output {
		for _, columnConfig := range reportOutput.DependentColumns {
			if columnConfig.OperationType == models.Average {
				calculateAverages(data, &columnConfig)
			}
		}
	}
	reportOutput.Results, err = convertToChartFormat(reportOutput.IndependentColumn, &output)
	if err != nil {
		return err
	}

	// Seek to the beginning of the file to allow for reading again
	_, err = csvFile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

func readCSVFromHandler(fileHandler *os.File) ([][]string, error) {
	_, err := fileHandler.Seek(0, 0)
	if err != nil {
		return nil, fmt.Errorf("error seeking in file: %v", err)
	}
	reader := csv.NewReader(fileHandler)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV: %v", err)
	}
	return records, nil
}

func incrementValue(output map[string]interface{}, key string) {
	if _, exists := output[key]; !exists {
		output[key] = 1
	} else {
		output[key] = output[key].(int) + 1
	}
}

func addNumericalValue(output map[string]interface{}, value string, key string) error {
	num, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("error parsing numerical value: %w", err)
	}
	if _, exists := output[key]; !exists {
		output[key] = num
	} else {
		output[key] = output[key].(int) + num
	}
	return nil
}

func addNumericalValueForAverage(output map[string]interface{}, value string, key string) error {
	num, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("error parsing numerical value: %w", err)
	}
	totalKey := key + "_total"
	countKey := key + "_count"
	if _, exists := output[totalKey]; !exists {
		output[totalKey] = 0
	}
	if _, exists := output[countKey]; !exists {
		output[countKey] = 0
	}
	output[totalKey] = output[totalKey].(int) + num
	output[countKey] = output[countKey].(int) + 1
	return nil
}

func calculateAverages(output map[string]interface{}, columnConfig *models.ReportOneDimConfig) {
	for key, value := range output {
		if strings.HasSuffix(key, "_total") {
			total := value.(int)
			countKey := strings.TrimSuffix(key, "_total") + "_count"
			count := output[countKey].(int)
			average := float64(total) / float64(count)
			roundedAverage := math.Round(average*1000) / 1000
			avgKey := strings.TrimSuffix(key, "_total")
			output[avgKey] = roundedAverage
			delete(output, key)
			delete(output, countKey)
		}
	}
}

func rowPassesFilters(row []string, filterColumns map[string][]string, headers []string) bool {
	if len(filterColumns) == 0 {
		return true
	}
	for column, filterValues := range filterColumns {
		if len(filterValues) == 0 {
			continue
		}

		colIndex := -1
		for i, header := range headers {
			if header == column {
				colIndex = i
				break
			}
		}
		if colIndex == -1 {
			return false
		}
		value := row[colIndex]
		valuePasses := false
		for _, filterValue := range filterValues {
			if value == filterValue {
				valuePasses = true
				break
			}
		}
		if !valuePasses {
			return false
		}
	}
	return true
}

func processOperation(record []string, output map[string]interface{}, columnConfig *models.ReportOneDimConfig, columnIndex int, headers []string, isOneDimension bool) error {
	dynamicFilters := columnConfig.FilterColumns
	if len(columnConfig.AcceptedValues) > 0 {
		if dynamicFilters == nil {
			dynamicFilters = make(map[string][]string)
		}
		dynamicFilters[columnConfig.Column] = columnConfig.AcceptedValues
	}
	if !rowPassesFilters(record, dynamicFilters, headers) {
		return nil // Skip this row, but don't return an error.
	}
	yValue := record[columnIndex]
	switch columnConfig.OperationType {
	case models.UniqueOccurrences:
		if isOneDimension {
			incrementValue(output, columnConfig.AggregateValueLabel)
		} else {
			incrementValue(output, yValue)
		}
	case models.Average:
		if columnConfig.AggregateValueLabel == "" {
			return fmt.Errorf("aggregate value cannot be unassigned for operation type: %s", columnConfig.OperationType)
		}
		if err := addNumericalValueForAverage(output, yValue, columnConfig.AggregateValueLabel); err != nil {
			return err
		}
	case models.NumericalSum:
		if err := addNumericalValue(output, yValue, columnConfig.AggregateValueLabel); err != nil {
			return err
		}
	case models.SetElementOccurrences:
		if columnConfig.AggregateValueLabel == "" {
			return fmt.Errorf("aggregate value cannot be unassigned for operation type: %s", columnConfig.OperationType)
		}
		incrementValue(output, columnConfig.AggregateValueLabel)
	default:
		return fmt.Errorf("unsupported operation type: %s", columnConfig.OperationType)
	}
	return nil
}

func findColumnIndex(headers []string, columnName string) int {
	for i, header := range headers {
		if header == columnName {
			return i
		}
	}
	return -1
}

func convertToChartFormat(IndependentColumn string, output *map[string]map[string]interface{}) ([]map[string]interface{}, error) {
	var transformedData []map[string]interface{}
	for key, value := range *output {
		if key == "" {
			return nil, fmt.Errorf("found empty key in output map, which is not allowed")
		}
		transformedMap := make(map[string]interface{})
		for k, v := range value {
			transformedMap[k] = v
		}
		transformedMap[IndependentColumn] = key
		transformedData = append(transformedData, transformedMap)
	}
	sortedData, err := sortData(IndependentColumn, transformedData)
	if err != nil {
		return nil, err
	}
	return sortedData, nil
}

func sortData(sortKey string, listOfMaps []map[string]interface{}) ([]map[string]interface{}, error) {
	for _, m := range listOfMaps {
		if _, ok := m[sortKey]; !ok {
			return nil, fmt.Errorf("sort key %s not found in one of the maps", sortKey)
		}
	}
	sort.Slice(listOfMaps, func(i, j int) bool {
		return listOfMaps[i][sortKey].(string) < listOfMaps[j][sortKey].(string)
	})
	return listOfMaps, nil
}
