//go:build windows

// Package process содержит проверку Windows Job Objects для Этапа 0.
// Это ещё не download-engine: вывод пишется в файлы, progress/pipes не реализованы.
package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const procThreadAttributeJobList = 0x0002000d

// RunInJob запускает executable напрямую с отдельными аргументами.
// JOB_LIST привязывает процесс атомарно при создании, до выполнения его кода.
// Дочерние процессы наследуют job; сам job handle не наследуется.
// Возвращается только после остановки ВСЕГО дерева (включая обычный exit родителя).
func RunInJob(ctx context.Context, executable string, args []string, dir string, stdout, stderr *os.File) (exitCode uint32, resultErr error) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return 0, err
	}
	defer stdin.Close()
	return RunInJobWithInput(ctx, executable, args, dir, stdin, stdout, stderr)
}

// RunInJobWithInput keeps transient input in an anonymous pipe, never argv/files.
// The caller owns stdin; process tree ownership is identical to RunInJob.
func RunInJobWithInput(ctx context.Context, executable string, args []string, dir string, stdin, stdout, stderr *os.File) (exitCode uint32, resultErr error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !filepath.IsAbs(executable) || stdin == nil || stdout == nil || stderr == nil {
		return 0, errors.New("Нужны абсолютный путь программы и файлы stdout/stderr")
	}
	application, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return 0, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, args...)))
	if err != nil {
		return 0, err
	}
	var directory *uint16
	if dir != "" {
		directory, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return 0, err
		}
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("Создание Job Object: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(job)) }()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return 0, err
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return 0, err
	}
	defer attributes.Delete()
	if err := attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return 0, err
	}
	// Наследуются только эти три дубликата; не handles других параллельных job.
	handles := make([]windows.Handle, 3)
	for i, f := range []*os.File{stdin, stdout, stderr} {
		if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(f.Fd()), windows.CurrentProcess(), &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return 0, err
		}
		defer windows.CloseHandle(handles[i])
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return 0, err
	}
	si := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{
		Cb:       uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		Flags:    windows.STARTF_USESTDHANDLES,
		StdInput: handles[0], StdOutput: handles[1], StdErr: handles[2],
	}, ProcThreadAttributeList: attributes.List()}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(application, commandLine, nil, nil, true,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, directory, &si.StartupInfo, &pi); err != nil {
		return 0, fmt.Errorf("Запуск процесса: %w", err)
	}
	defer windows.CloseHandle(pi.Process)
	windows.CloseHandle(pi.Thread)
	defer func() {
		// TerminateJobObject асинхронен: освобождение worker допустимо после ActiveProcesses=0.
		resultErr = errors.Join(resultErr, windows.TerminateJobObject(job, 1), waitForEmptyJob(job))
	}()
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		status, err := windows.WaitForSingleObject(pi.Process, 20)
		if err != nil {
			return 0, err
		}
		if status == windows.WAIT_OBJECT_0 {
			if err := windows.GetExitCodeProcess(pi.Process, &exitCode); err != nil {
				return 0, err
			}
			if exitCode != 0 {
				return exitCode, fmt.Errorf("Процесс завершился с кодом %d", exitCode)
			}
			return exitCode, nil
		}
	}
}

// Структура JOBOBJECT_BASIC_ACCOUNTING_INFORMATION из winnt.h.
type jobAccounting struct {
	TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime               int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses uint32
}

func waitForEmptyJob(job windows.Handle) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		var info jobAccounting
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
			return err
		}
		if info.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("Дерево процессов не завершилось за 5 секунд")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
