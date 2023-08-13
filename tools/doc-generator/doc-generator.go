package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"

	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/components"

	_ "kubevirt.io/kubevirt/pkg/monitoring/configuration"
	domainstats "kubevirt.io/kubevirt/pkg/monitoring/domainstats/prometheus" // import for prometheus metrics
	_ "kubevirt.io/kubevirt/pkg/virt-controller/watch"
)

// constant parts of the file
const (
	genFileComment = `<!--
	This is an auto-generated file.
	PLEASE DO NOT EDIT THIS FILE.
	See "Developing new metrics" below how to generate this file
-->`
	title      = "# KubeVirt metrics\n"
	background = "This document aims to help users that are not familiar with all metrics exposed by different KubeVirt components.\n" +
		"All metrics documented here are auto-generated by the utility tool `tools/doc-generator` and reflects exactly what is being exposed.\n\n"

	KVSpecificMetrics = "## KubeVirt Metrics List\n" +
		"### kubevirt_info\n" +
		"Version information.\n\n"

	opening = genFileComment + "\n\n" +
		title +
		background +
		KVSpecificMetrics

	// footer
	footerHeading = "## Developing new metrics\n"
	footerContent = "After developing new metrics or changing old ones, please run `make generate` to regenerate this document.\n\n" +
		"If you feel that the new metric doesn't follow these rules, please change `doc-generator` with your needs.\n"

	footer = footerHeading + footerContent
)

func main() {
	handler := domainstats.Handler(1)
	RegisterFakeDomainCollector()
	RegisterFakeVMCollector()
	RegisterFakeMigrationsCollector()

	req, err := http.NewRequest(http.MethodGet, "/metrics", nil)
	checkError(err)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	metrics := getMetricsNotIncludeInEndpointByDefault()

	if status := recorder.Code; status == http.StatusOK {
		err := parseVirtMetrics(recorder.Body, &metrics)
		checkError(err)

	} else {
		panic(fmt.Errorf("got HTTP status code of %d from /metrics", recorder.Code))
	}
	writeToFile(metrics)
}

func writeToFile(metrics metricList) {
	newFile, err := os.Create("newmetrics.md")
	checkError(err)
	defer newFile.Close()

	fmt.Fprint(newFile, opening)
	metrics.writeToFile(newFile)

	fmt.Fprint(newFile, footer)

}

type metric struct {
	name        string
	description string
	mType       string
}

func (m metric) writeToFile(newFile io.WriteCloser) {
	fmt.Fprintln(newFile, "###", m.name)
	fmt.Fprintln(newFile, m.description, "Type:", m.mType+".")
	fmt.Fprintln(newFile)
}

type metricList []metric

// Len implements sort.Interface.Len
func (m metricList) Len() int {
	return len(m)
}

// Less implements sort.Interface.Less
func (m metricList) Less(i, j int) bool {
	return m[i].name < m[j].name
}

// Swap implements sort.Interface.Swap
func (m metricList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m metricList) writeToFile(newFile io.WriteCloser) {
	for _, met := range m {
		met.writeToFile(newFile)
	}
}

func getMetricsNotIncludeInEndpointByDefault() metricList {
	metrics := metricList{
		{
			name:        domainstats.MigrateVmiDataProcessedMetricName,
			description: "The total Guest OS data processed and migrated to the new VM.",
			mType:       "Gauge",
		},
		{
			name:        domainstats.MigrateVmiDataRemainingMetricName,
			description: "The remaining guest OS data to be migrated to the new VM.",
			mType:       "Gauge",
		},
		{
			name:        domainstats.MigrateVmiDirtyMemoryRateMetricName,
			description: "The rate of memory being dirty in the Guest OS.",
			mType:       "Gauge",
		},
		{
			name:        domainstats.MigrateVmiMemoryTransferRateMetricName,
			description: "The rate at which the memory is being transferred.",
			mType:       "Gauge",
		},
		{
			name:        domainstats.MigrateVmiDiskTransferRateMetricName,
			description: "The rate at which the disk is being transferred.",
			mType:       "Gauge",
		},
		{
			name:        "kubevirt_vmi_phase_count",
			description: "Sum of VMIs per phase and node. `phase` can be one of the following: [`Pending`, `Scheduling`, `Scheduled`, `Running`, `Succeeded`, `Failed`, `Unknown`].",
			mType:       "Gauge",
		},
		{
			name:        "kubevirt_vmi_non_evictable",
			description: "Indication for a VirtualMachine that its eviction strategy is set to Live Migration but is not migratable.",
			mType:       "Gauge",
		},
		{
			name:        "kubevirt_vmi_migration_phase_transition_time_from_creation_seconds",
			description: "Histogram of VM migration phase transitions duration from creation time in seconds.",
			mType:       "Histogram",
		},
		{
			name:        "kubevirt_vmi_phase_transition_time_seconds",
			description: "Histogram of VM phase transitions duration between different phases in seconds.",
			mType:       "Histogram",
		},
		{
			name:        "kubevirt_vmi_phase_transition_time_from_creation_seconds",
			description: "Histogram of VM phase transitions duration from creation time in seconds.",
			mType:       "Histogram",
		},
		{
			name:        "kubevirt_vmi_phase_transition_time_from_deletion_seconds",
			description: "Histogram of VM phase transitions duration from deletion time in seconds.",
			mType:       "Histogram",
		},
		{
			name:        "kubevirt_virt_operator_leading_status",
			description: "Indication for an operating virt-operator.",
			mType:       "Gauge",
		},
		{
			name:        "kubevirt_virt_operator_ready_status",
			description: "Indication for a virt-operator that is ready to take the lead.",
			mType:       "Gauge",
		},
	}

	for _, rule := range components.GetRecordingRules("") {
		metrics = append(metrics, metric{
			name:        rule.Rule.Record,
			description: rule.Description,
			mType:       strings.Title(string(rule.MType)),
		})
	}

	return metrics
}

func parseMetricDesc(line string) (string, string) {
	split := strings.Split(line, " ")
	name := split[2]
	split[3] = strings.Title(split[3])
	description := strings.Join(split[3:], " ")
	return name, description
}

func parseMetricType(scan *bufio.Scanner, name string) string {
	for scan.Scan() {
		typeLine := scan.Text()
		if strings.HasPrefix(typeLine, "# TYPE ") {
			split := strings.Split(typeLine, " ")
			if split[2] == name {
				return strings.Title(split[3])
			}
		}
	}
	return ""
}

const filter = "kubevirt_"

func parseVirtMetrics(r io.Reader, metrics *metricList) error {
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		helpLine := scan.Text()
		if strings.HasPrefix(helpLine, "# HELP ") {
			if strings.Contains(helpLine, filter) {
				metName, metDesc := parseMetricDesc(helpLine)
				metType := parseMetricType(scan, metName)
				*metrics = append(*metrics, metric{name: metName, description: metDesc, mType: metType})
			}
		}
	}

	if scan.Err() != nil {
		return fmt.Errorf("failed to parse metrics from prometheus endpoint, %w", scan.Err())
	}

	sort.Sort(metrics)

	return nil
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}
