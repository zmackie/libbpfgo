package main

import "C"

import (
	"os"
	"time"
	"unsafe"

	"encoding/binary"
	"fmt"
	"syscall"

	bpf "github.com/aquasecurity/libbpfgo"
)

func resizeMap(module *bpf.Module, name string, size uint32) error {
	m, err := module.GetMap("events")
	if err != nil {
		return err
	}

	if err = m.Resize(size); err != nil {
		return err
	}

	if actual := m.GetMaxEntries(); actual != size {
		return fmt.Errorf("map resize failed, expected %v, actual %v", size, actual)
	}

	return nil
}

func main() {

	bpfModule, err := bpf.NewModuleFromFile("main.bpf.o")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
	defer bpfModule.Close()

	if err = resizeMap(bpfModule, "events", 8192); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	bpfModule.BPFLoadObject()
	prog, err := bpfModule.GetProgram("kprobe__sys_mmap")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	testerMap, err := bpfModule.GetMap("tester")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	key1 := uint32(1)
	value1 := struct{ x int }{50}
	key1Unsafe := unsafe.Pointer(&key1)
	value1Unsafe := unsafe.Pointer(&value1)
	testerMap.Update(key1Unsafe, value1Unsafe)

	key2 := int64(42069420)
	value2 := []byte{'a', 'b', 'c'}
	key2Unsafe := unsafe.Pointer(&key2)
	value2Unsafe := unsafe.Pointer(&value2[0])
	testerMap.Update(key2Unsafe, value2Unsafe)

	_, err = prog.AttachKprobe("__x64_sys_mmap")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	eventsChannel := make(chan []byte)
	lostChannel := make(chan uint64)
	pb, err := bpfModule.InitPerfBuf("events", eventsChannel, lostChannel, 1)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	pb.Start()

	go func() {
		time.Sleep(time.Second)
		syscall.Mmap(999, 999, 999, 1, 1)
		syscall.Mmap(999, 999, 999, 1, 1)
	}()

	ev := <-eventsChannel
	if binary.LittleEndian.Uint32(ev) != 50 {
		fmt.Fprintf(os.Stderr, "invalid data retrieved\n")
		os.Exit(-1)
	}

	ev = <-eventsChannel
	if ev[0] != value2[0] || ev[1] != value2[1] || ev[2] != value2[2] {
		fmt.Fprintf(os.Stderr, "invalid data retrieved\n")
		os.Exit(-1)
	}

	pb.Stop()
	pb.Close()
}
