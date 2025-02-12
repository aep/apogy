package reactor

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	wapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/assemblyscript"
)

type running struct {
	ctx    context.Context
	cancel func()
	wr     wazero.Runtime
	mod    wapi.Module
}

func startAssemblyScript(wasm []byte) (*running, error) {

	ctx, cancel := context.WithCancel(context.Background())

	config := wazero.NewRuntimeConfig()
	config.WithMemoryLimitPages(1000) // 64MB
	config.WithCloseOnContextDone(true)
	wr := wazero.NewRuntimeWithConfig(ctx, config)
	retok := false
	defer func() {
		if !retok {
			cancel()
			wr.Close(ctx)
		}
	}()

	builder := wr.NewHostModuleBuilder("env")
	assemblyscript.NewFunctionExporter().WithTraceToStderr().ExportFunctions(builder)
	_, err := builder.Instantiate(ctx)
	if err != nil {
		return nil, err
	}

	mod, err := wr.InstantiateWithConfig(ctx, wasm,
		wazero.NewModuleConfig().WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		return nil, err
	}

	newFn := mod.ExportedFunction("__new")
	if newFn == nil {
		return nil, fmt.Errorf("wasm does not export __new. forgot --exportRuntime ?")
	}
	pinFn := mod.ExportedFunction("__pin")
	if pinFn == nil {
		return nil, fmt.Errorf("wasm does not export __pin. forgot --exportRuntime ?")
	}
	unpinFn := mod.ExportedFunction("__unpin")
	if unpinFn == nil {
		return nil, fmt.Errorf("wasm does not export __unpin. forgot --exportRuntime ?")
	}
	collectFn := mod.ExportedFunction("__collect")
	if collectFn == nil {
		return nil, fmt.Errorf("wasm does not export __collect. forgot --exportRuntime ?")
	}

	retok = true

	return &running{
		ctx:    ctx,
		wr:     wr,
		cancel: cancel,
		mod:    mod,
	}, nil
}

func (self *running) copyStringToGuest(ctx context.Context, s string) (uint64, error) {
	newFn := self.mod.ExportedFunction("__new")
	if newFn == nil {
		return 0, fmt.Errorf("wasm does not export __new. forgot --exportRuntime ?")
	}

	const STRING_ID = 2

	utf16Slice := utf16.Encode([]rune(s))

	results, err := newFn.Call(ctx, uint64(2*len(utf16Slice)), STRING_ID)
	if err != nil {
		return 0, err
	}

	guestPtr := results[0]
	if guestPtr == 0 {
		return 0, errors.New("malloc failed")
	}

	for i, v := range utf16Slice {

		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)

		if !self.mod.Memory().Write(uint32(guestPtr+uint64(i*2)), b[:]) {
			return 0, fmt.Errorf("Memory.Write(%d, %d) out of range of memory size %d",
				guestPtr+uint64(i*2), len(s), self.mod.Memory().Size())
		}
	}

	return guestPtr, nil
}

func (self *running) copyStringFromGuest(ctx context.Context, ptr uint64) (string, error) {

	sizeBytes, ok := self.mod.Memory().Read(uint32(ptr-4), 4)
	if !ok {
		return "", fmt.Errorf("failed reading string size offset from wasm mem")
	}

	size := binary.LittleEndian.Uint32(sizeBytes)

	if size%2 != 0 {
		return "", fmt.Errorf("invalid UTF-16 bytes: length must be even")
	}

	b, ok := self.mod.Memory().Read(uint32(ptr), size)
	if !ok {
		return "", fmt.Errorf("failed reading string from wasm mem")
	}

	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		u16[i/2] = binary.LittleEndian.Uint16(b[i:])
	}

	runes := utf16.Decode(u16)

	return string(runes), nil
}

func (self *running) validate(ctx context.Context, oldJson string, nuwJson string) error {

	validateFn := self.mod.ExportedFunction("validate")
	if validateFn == nil {
		return fmt.Errorf("wasm does not export validate")
	}

	newFn := self.mod.ExportedFunction("__new")
	if newFn == nil {
		return fmt.Errorf("wasm does not export __new. forgot --exportRuntime ?")
	}
	pinFn := self.mod.ExportedFunction("__pin")
	if pinFn == nil {
		return fmt.Errorf("wasm does not export __pin. forgot --exportRuntime ?")
	}
	unpinFn := self.mod.ExportedFunction("__unpin")
	if unpinFn == nil {
		return fmt.Errorf("wasm does not export __unpin. forgot --exportRuntime ?")
	}
	collectFn := self.mod.ExportedFunction("__collect")
	if collectFn == nil {
		return fmt.Errorf("wasm does not export __collect. forgot --exportRuntime ?")
	}

	defer collectFn.Call(ctx)

	jsonAGuestPtr, err := self.copyStringToGuest(ctx, oldJson)
	if err != nil {
		return nil
	}
	pinFn.Call(ctx, jsonAGuestPtr)
	defer unpinFn.Call(ctx, jsonAGuestPtr)

	jsonBGuestPtr, err := self.copyStringToGuest(ctx, nuwJson)
	if err != nil {
		return nil
	}
	pinFn.Call(ctx, jsonBGuestPtr)
	defer unpinFn.Call(ctx, jsonBGuestPtr)

	vrr, err := validateFn.Call(ctx, jsonAGuestPtr, jsonBGuestPtr)
	if err != nil {
		return err
	}

	res, err := self.copyStringFromGuest(ctx, vrr[0])
	if err != nil {
		return err
	}

	if res != "" {
		return errors.New(res)
	}

	return nil
}
