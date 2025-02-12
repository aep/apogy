package main

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/assemblyscript"
)

// greetWasm was compiled using `zig build`
//
//go:embed validator.wasm
var greetWasm []byte

// main shows how to interact with a WebAssembly function that was compiled from Zig.
//
// See README.md for a full description.
func main() {
	if err := run(); err != nil {
		log.Panicln(err)
	}
}

func run() error {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.~

	builder := r.NewHostModuleBuilder("env")
	assemblyscript.NewFunctionExporter().WithTraceToStderr().ExportFunctions(builder)
	_, err := builder.Instantiate(ctx)
	if err != nil {
		return err
	}

	mod, err := r.InstantiateWithConfig(ctx, greetWasm,
		wazero.NewModuleConfig().WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		return err
	}

	validateFn := mod.ExportedFunction("validate")

	newFn := mod.ExportedFunction("__new")
	if newFn == nil {
		return fmt.Errorf("wasm does not export __new. forgot --exportRuntime ?")
	}
	pinFn := mod.ExportedFunction("__pin")
	unpinFn := mod.ExportedFunction("__unpin")
	collectFn := mod.ExportedFunction("__collect")

	defer collectFn.Call(ctx)

	jsonA := `
	{
	  "history": {
	    "created": "2025-02-11T14:37:23.620130901+01:00",
	    "updated": "2025-02-11T18:08:45.077903279+01:00"
	  },
	  "id": "bigWinner2025",
	  "model": "com.example.EmailTemplate",
	  "val": {
	    "body": "quadrillion us dolors just send us your SSN quick",
	    "subject": "you win big",
	    "to": [
	      "mikemikeson@example.com",
	      "spam-me-daddy@example.com"
	    ],
	  },
	  "version": 38
	}
	`
	jsonB := `
	{
	  "history": {
	    "created": "2025-02-11T14:37:23.620130901+01:00",
	    "updated": "2025-02-11T18:08:45.077903279+01:00"
	  },
	  "id": "bigWinner2025",
	  "model": "com.example.EmailTemplate",
	  "val": {
	    "body": "quadrillion us dolors just send us your SSN quick",
	    "subject": "you win big",
	    "to": [
	      "mikemikeson@example.com",
	      "spam-me-daddy@example.com"
	    ]
	  },
	  "version": 38
	}
	`

	copyStringToGuest := func(s string) (uint64, error) {

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

			if !mod.Memory().Write(uint32(guestPtr+uint64(i*2)), b[:]) {
				return 0, fmt.Errorf("Memory.Write(%d, %d) out of range of memory size %d",
					guestPtr+uint64(i*2), len(s), mod.Memory().Size())
			}
		}

		return guestPtr, nil
	}

	jsonAGuestPtr, err := copyStringToGuest(jsonA)
	if err != nil {
		return nil
	}
	pinFn.Call(ctx, jsonAGuestPtr)
	defer unpinFn.Call(ctx, jsonAGuestPtr)

	jsonBGuestPtr, err := copyStringToGuest(jsonB)
	if err != nil {
		return nil
	}
	pinFn.Call(ctx, jsonBGuestPtr)
	defer unpinFn.Call(ctx, jsonBGuestPtr)

	vrr, err := validateFn.Call(ctx, jsonAGuestPtr, jsonBGuestPtr)
	if err != nil {
		return err
	}

	fmt.Println("validate returned >>", vrr)

	return nil
}

func host_log(_ context.Context, m api.Module, offset, byteCount uint32) {
	buf, ok := m.Memory().Read(offset, byteCount)
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", offset, byteCount)
	}
	fmt.Println("[wasm] ", string(buf))
}
