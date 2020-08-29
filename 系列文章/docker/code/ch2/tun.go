package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/songgao/water"
)

type Interface struct {
	tun *water.Interface
}

func NewInterface() (*Interface, error) {
	iface := &Interface{}

	ifconfig := water.Config{
		DeviceType: water.TUN,
	}

	ifce, err := water.New(ifconfig)
	if err != nil {
		return nil, err
	}

	iface.tun = ifce
	return iface, nil
}

func (iface *Interface) Up() error {
	switch runtime.GOOS {
	case "linux", "darwin":
		out, err := execCmd("ifconfig", []string{iface.tun.Name(), "up"})
		if err != nil {
			return fmt.Errorf("ifconfig fail: %s %v", out, err)
		}

	default:
		return fmt.Errorf("unsupported: %s %s", runtime.GOOS, runtime.GOARCH)

	}

	return nil
}

func (iface *Interface) Read() ([]byte, error) {
	buf := make([]byte, 2048)
	n, err := iface.tun.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

func (iface *Interface) Write(buf []byte) (int, error) {
	return iface.tun.Write(buf)
}

func (iface *Interface) Close() {
	iface.tun.Close()
}

func execCmd(cmd string, args []string) (string, error) {
	b, err := exec.Command(cmd, args...).CombinedOutput()
	return string(b), err
}

func main() {
	iface, err := NewInterface()
	if err != nil {
		fmt.Println("[E] new interface fail: ", err)
		return
	}

	defer iface.Close()
	iface.Up()
	for {
		buf, err := iface.Read()
		if err != nil {
			fmt.Printf("[E] read iface fail: %v\n", err)
			break
		}
		fmt.Println(buf)
	}
}
