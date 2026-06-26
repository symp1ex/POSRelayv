//go:build windows

package gui

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"posrelayd-viewer/internal/logger"
)

const (
	jobObjectInfoClassExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose               = 0x00002000
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var sessionJob windows.Handle

func ensureSessionJob() error {
	if sessionJob != 0 {
		return nil
	}

	logger.Posrelayv.Debug("[GUI] Creating session job object")

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to create session job object: %v", err)
		return fmt.Errorf("CreateJobObject failed: %w", err)
	}

	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose

	ret, err := windows.SetInformationJobObject(
		job,
		jobObjectInfoClassExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if ret == 0 {
		windows.CloseHandle(job)
		logger.Posrelayv.Errorf("[GUI] Failed to configure session job object: %v", err)
		return fmt.Errorf("SetInformationJobObject failed: %w", err)
	}

	sessionJob = job
	logger.Posrelayv.Debug("[GUI] Session job object created")
	return nil
}

func addProcessToSessionJob(process windows.Handle) error {
	if err := ensureSessionJob(); err != nil {
		return err
	}

	if err := windows.AssignProcessToJobObject(sessionJob, process); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to assign process to session job: %v", err)
		return fmt.Errorf("AssignProcessToJobObject failed: %w", err)
	}

	logger.Posrelayv.Debug("[GUI] Process assigned to session job")
	return nil
}

func closeSessionJob() {
	if sessionJob == 0 {
		return
	}

	windows.CloseHandle(sessionJob)
	sessionJob = 0
	logger.Posrelayv.Debug("[GUI] Session job object closed")
}
