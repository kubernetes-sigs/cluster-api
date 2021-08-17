package controllers

import (
	"sync/atomic"
)

type InstallStatus int32

const (
	InstallStatusUnknown InstallStatus = iota
	InstallStatusInstalling
	InstallStatusReady
)

func (i InstallStatus) String() string {
	switch i {
	case InstallStatusInstalling:
		return "Installing"
	case InstallStatusReady:
		return "Ready"
	default:
		return "Unknown"
	}
}

type singletonInstaller struct {
	status *InstallStatus
}

type InstallFn func() error

// SingletonInstaller is a simple interface to ensure that only one thread
// can run an install at a time.
type SingletonInstaller interface {
	Install(InstallFn) (InstallStatus, error)
}

func NewSingletonInstaller() SingletonInstaller {
	si := &singletonInstaller{status: new(InstallStatus)}
	atomic.StoreInt32((*int32)(si.status), int32(InstallStatusUnknown))
	return si
}

func (s *singletonInstaller) getStatus() InstallStatus {
	return InstallStatus(atomic.LoadInt32((*int32)(s.status)))
}

// compareAndSwap will return false if oldStatus is not as expected.
func (s *singletonInstaller) setStatus(oldStatus, newStatus InstallStatus) bool {
	return atomic.CompareAndSwapInt32((*int32)(s.status), int32(oldStatus), int32(newStatus))
}

func (s *singletonInstaller) Install(installFn InstallFn) (InstallStatus, error) {
	for {
		status := s.getStatus()
		if status == InstallStatusReady || status == InstallStatusInstalling {
			// nothing to do as we are in either of:
			// 1. Ready: already installed
			// 2. Installing: another thread is busy installing
			return status, nil
		}
		// we must definitally own the "installing" status before beginning the Install.
		// if another thread has caused this to be in a different status then we need
		// to go around the loop again.
		if s.setStatus(InstallStatusUnknown, InstallStatusInstalling) {
			break
		}
	}

	err := installFn()

	// any thread can start the install, but only we can exit it
	// (not checking the return of setStatus).
	if err != nil {
		s.setStatus(InstallStatusInstalling, InstallStatusUnknown)
	} else {
		s.setStatus(InstallStatusInstalling, InstallStatusReady)
	}
	return s.getStatus(), err
}
