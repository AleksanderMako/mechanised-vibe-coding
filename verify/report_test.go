package verify_test

import (
	"slices"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/verify"
)

func TestOrder_IsTheFixedStepOrder(t *testing.T) {
	want := []verify.StepName{"gofmt", "vendor", "build", "vet", "test", "lint"}
	if !slices.Equal(verify.Order, want) {
		t.Errorf("Order: want %v, got %v", want, verify.Order)
	}
}

func TestReport_OKWhenEveryStepPassedOrWasSkipped(t *testing.T) {
	report := verify.Report{Steps: []verify.Step{
		{Name: verify.StepFormat, OK: true},
		{Name: verify.StepVendor, Skipped: true},
		{Name: verify.StepBuild, OK: true},
		{Name: verify.StepVet, OK: true},
		{Name: verify.StepTest, OK: true},
		{Name: verify.StepLint, Skipped: true},
	}}
	if !report.OK() {
		t.Error("OK: want true when no step failed")
	}
	if failed := report.Failed(); failed != nil {
		t.Errorf("Failed: want nil, got %+v", *failed)
	}
}

func TestReport_FailedReturnsFirstFailedStep(t *testing.T) {
	report := verify.Report{Steps: []verify.Step{
		{Name: verify.StepFormat, OK: true},
		{Name: verify.StepVendor, Skipped: true},
		{Name: verify.StepBuild, OK: false, Output: "build broke"},
		{Name: verify.StepVet, OK: false, Output: "vet broke"},
		{Name: verify.StepTest, Skipped: true},
		{Name: verify.StepLint, Skipped: true},
	}}
	if report.OK() {
		t.Error("OK: want false when a step failed")
	}
	failed := report.Failed()
	if failed == nil {
		t.Fatal("Failed: want the build step, got nil")
	}
	if failed.Name != verify.StepBuild || failed.Output != "build broke" {
		t.Errorf("Failed: want the build step, got %+v", *failed)
	}
}

func TestReport_SkippedStepsAreNotFailures(t *testing.T) {
	report := verify.Report{Steps: []verify.Step{
		{Name: verify.StepFormat, Skipped: true},
		{Name: verify.StepVendor, Skipped: true},
		{Name: verify.StepBuild, Skipped: true},
		{Name: verify.StepVet, Skipped: true},
		{Name: verify.StepTest, Skipped: true},
		{Name: verify.StepLint, Skipped: true},
	}}
	if !report.OK() {
		t.Error("OK: want true when every step was skipped")
	}
	if failed := report.Failed(); failed != nil {
		t.Errorf("Failed: want nil, got %+v", *failed)
	}
}
