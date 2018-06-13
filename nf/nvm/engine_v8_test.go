// Copyright (C) 2017 go-nebulas authors
//
// This file is part of the go-nebulas library.
//
// the go-nebulas library is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// the go-nebulas library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with the go-nebulas library.  If not, see <http://www.gnu.org/licenses/>.
//

package nvm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/nebulasio/go-nebulas/account"
	"github.com/nebulasio/go-nebulas/net"

	"github.com/nebulasio/go-nebulas/consensus/dpos"
	"github.com/nebulasio/go-nebulas/core/pb"
	"github.com/nebulasio/go-nebulas/neblet/pb"

	"github.com/nebulasio/go-nebulas/core"
	"github.com/nebulasio/go-nebulas/core/state"
	"github.com/nebulasio/go-nebulas/crypto"
	"github.com/nebulasio/go-nebulas/crypto/keystore"
	"github.com/nebulasio/go-nebulas/crypto/keystore/secp256k1"
	"github.com/nebulasio/go-nebulas/storage"
	"github.com/nebulasio/go-nebulas/util"
	"github.com/nebulasio/go-nebulas/util/byteutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contractStr = "n218MQSwc7hcXvM7rUkr6smMoiEf2VbGuYr"

func newUint128FromIntWrapper(a int64) *util.Uint128 {
	b, _ := util.NewUint128FromInt(a)
	return b
}

type testBlock struct {
}

// Coinbase mock
func (block *testBlock) Coinbase() *core.Address {
	addr, _ := core.AddressParse("n1FkntVUMPAsESuCAAPK711omQk19JotBjM")
	return addr
}

// Hash mock
func (block *testBlock) Hash() byteutils.Hash {
	return []byte("59fc526072b09af8a8ca9732dae17132c4e9127e43cf2232")
}

// Height mock
func (block *testBlock) Height() uint64 {
	return core.NvmMemoryLimitWithoutInjectHeight
}

// RandomSeed mock
func (block *testBlock) RandomSeed() string {
	return "59fc526072b09af8a8ca9732dae17132c4e9127e43cf2232"
}

// RandomAvailable mock
func (block *testBlock) RandomAvailable() bool {
	return true
}

// DateAvailable
func (block *testBlock) DateAvailable() bool {
	return true
}

// GetTransaction mock
func (block *testBlock) GetTransaction(hash byteutils.Hash) (*core.Transaction, error) {
	return nil, nil
}

// RecordEvent mock
func (block *testBlock) RecordEvent(txHash byteutils.Hash, topic, data string) error {
	return nil
}

func (block *testBlock) Timestamp() int64 {
	return int64(0)
}

func mockBlock() Block {
	block := &testBlock{}
	return block
}

func mockTransaction() *core.Transaction {
	return mockNormalTransaction("n1FkntVUMPAsESuCAAPK711omQk19JotBjM", "n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR", "0")
}

const ContractName = "contract.js"

func mockNormalTransaction(from, to, value string) *core.Transaction {

	fromAddr, _ := core.AddressParse(from)
	toAddr, _ := core.AddressParse(to)
	payload, _ := core.NewBinaryPayload(nil).ToBytes()
	gasPrice, _ := util.NewUint128FromString("1000000")
	gasLimit, _ := util.NewUint128FromString("2000000")
	v, _ := util.NewUint128FromString(value)
	tx, _ := core.NewTransaction(1, fromAddr, toAddr, v, 1, core.TxPayloadBinaryType, payload, gasPrice, gasLimit)

	priv1 := secp256k1.GeneratePrivateKey()
	signature, _ := crypto.NewSignature(keystore.SECP256K1)
	signature.InitSign(priv1)
	tx.Sign(signature)
	return tx
}

func TestRunScriptSource(t *testing.T) {
	tests := []struct {
		filepath       string
		expectedErr    error
		expectedResult string
	}{
		{"test/test_require.js", nil, "\"\""},
		{"test/test_console.js", nil, "\"\""},
		{"test/test_storage_handlers.js", nil, "\"\""},
		{"test/test_storage_class.js", nil, "\"\""},
		{"test/test_storage.js", nil, "\"\""},
		{"test/test_eval.js", core.ErrExecutionFailed, "EvalError: Code generation from strings disallowed for this context"},
		{"test/test_date.js", nil, "\"\""},
		{"test/test_bignumber_random.js", core.ErrExecutionFailed, "Error: BigNumber.random is not allowed in nvm."},
		{"test/test_random_enable.js", nil, "\"\""},
		{"test/test_random_disable.js", core.ErrExecutionFailed, "Error: Math.random func is not allowed in nvm."},
		{"test/test_random_seed.js", core.ErrExecutionFailed, "Error: input seed must be a string"},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(900000, 10000000)
			result, err := engine.RunScriptSource(string(data), 0)
			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expectedResult, result)
			engine.Dispose()
		})
	}
}

func TestRunScriptSourceInModule(t *testing.T) {
	tests := []struct {
		filepath    string
		sourceType  string
		expectedErr error
	}{
		{"./test/test_require.js", "js", nil},
		{"./test/test_setTimeout.js", "js", core.ErrExecutionFailed},
		{"./test/test_console.js", "js", nil},
		{"./test/test_storage_handlers.js", "js", nil},
		{"./test/test_storage_class.js", "js", nil},
		{"./test/test_storage.js", "js", nil},
		{"./test/test_ERC20.js", "js", nil},
		{"./test/test_eval.js", "js", core.ErrExecutionFailed},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(100000, 10000000)
			engine.AddModule(ContractName, string(data), 0)
			runnableSource := fmt.Sprintf("require(\"%s\");", ContractName)
			_, err = engine.RunScriptSource(runnableSource, 0)

			assert.Equal(t, tt.expectedErr, err)
			engine.Dispose()
		})
	}
}

func TestRunScriptSourceWithLimits(t *testing.T) {
	tests := []struct {
		name                          string
		filepath                      string
		limitsOfExecutionInstructions uint64
		limitsOfTotalMemorySize       uint64
		expectedErr                   error
	}{
		{"1", "test/test_oom_1.js", 100000, 0, ErrInsufficientGas},
		{"2", "test/test_oom_1.js", 0, 500000, ErrExceedMemoryLimits},
		{"3", "test/test_oom_1.js", 1000000, 50000000, ErrInsufficientGas},
		{"4", "test/test_oom_1.js", 5000000, 70000, ErrExceedMemoryLimits},

		{"5", "test/test_oom_2.js", 100000, 0, ErrInsufficientGas},
		{"6", "test/test_oom_2.js", 0, 80000, ErrExceedMemoryLimits},
		{"7", "test/test_oom_2.js", 10000000, 10000000, ErrInsufficientGas},
		{"8", "test/test_oom_2.js", 10000000, 70000, ErrExceedMemoryLimits},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(100000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			// direct run.
			(func() {
				engine := NewV8Engine(ctx)
				engine.SetExecutionLimits(tt.limitsOfExecutionInstructions, tt.limitsOfTotalMemorySize)
				source, _, _ := engine.InjectTracingInstructions(string(data))
				_, err = engine.RunScriptSource(source, 0)
				fmt.Printf("err:%v\n", err)
				assert.Equal(t, tt.expectedErr, err)
				engine.Dispose()
			})()

			// modularized run.
			(func() {
				moduleID := fmt.Sprintf("%s", ContractName)
				runnableSource := fmt.Sprintf("require(\"%s\");", moduleID)

				engine := NewV8Engine(ctx)
				engine.SetExecutionLimits(tt.limitsOfExecutionInstructions, tt.limitsOfTotalMemorySize)
				engine.AddModule(ContractName, string(data), 0)
				_, err = engine.RunScriptSource(runnableSource, 0)
				assert.Equal(t, tt.expectedErr, err)
				engine.Dispose()
			})()
		})
	}
}

func TestRunScriptSourceMemConsistency(t *testing.T) {
	tests := []struct {
		name                          string
		filepath                      string
		limitsOfExecutionInstructions uint64
		limitsOfTotalMemorySize       uint64
		expectedMem                   uint64
	}{
		{"3", "test/test_oom_3.js", 1000000000, 5000000000, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(100000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			// direct run.
			(func() {
				engine := NewV8Engine(ctx)
				engine.SetExecutionLimits(tt.limitsOfExecutionInstructions, tt.limitsOfTotalMemorySize)
				source, _, _ := engine.InjectTracingInstructions(string(data))
				_, err = engine.RunScriptSource(source, 0)
				// assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, err)
				engine.Dispose()
			})()

			// modularized run.
			(func() {
				moduleID := fmt.Sprintf("%s", ContractName)
				runnableSource := fmt.Sprintf("require(\"%s\");", moduleID)

				engine := NewV8Engine(ctx)
				engine.SetExecutionLimits(tt.limitsOfExecutionInstructions, tt.limitsOfTotalMemorySize)
				engine.AddModule(ContractName, string(data), 0)
				_, err = engine.RunScriptSource(runnableSource, 0)
				// assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, err)
				engine.CollectTracingStats()
				// fmt.Printf("total:%v", engine.actualTotalMemorySize)
				assert.Equal(t, uint64(6703104), engine.actualTotalMemorySize)
				engine.Dispose()
			})()
		})
	}
}

func TestV8ResourceLimit(t *testing.T) {
	tests := []struct {
		name          string
		contractPath  string
		sourceType    string
		initArgs      string
		callArgs      string
		initExceptErr string
		callExceptErr string
	}{
		{"deploy test_oom_4.js", "./test/test_oom_4.js", "js", "[31457280]", "[31457280]", "", ""},
		{"deploy test_oom_4.js", "./test/test_oom_4.js", "js", "[37748736]", "[37748736]", "", ""},
		{"deploy test_oom_4.js", "./test/test_oom_4.js", "js", "[41943039]", "[41943039]", "exceed memory limits", "exceed memory limits"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			engine.CollectTracingStats()
			fmt.Printf("total:%v", engine.actualTotalMemorySize)
			// assert.Nil(t, err)
			if err != nil {
				fmt.Printf("err:%v", err.Error())
				assert.Equal(t, tt.initExceptErr, err.Error())
			} else {
				assert.Equal(t, tt.initExceptErr, "")
			}
			// assert.Equal(t, tt.initExceptErr, err.Error)

			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "newMem", tt.callArgs)
			// assert.Nil(t, err)
			// assert.Equal(t, tt.initExceptErr, err.Error)
			if err != nil {
				assert.Equal(t, tt.initExceptErr, err.Error())
			} else {
				assert.Equal(t, tt.initExceptErr, "")
			}
			engine.Dispose()

		})
	}
}
func TestRunScriptSourceTimeout(t *testing.T) {
	tests := []struct {
		filepath string
	}{
		{"test/test_infinite_loop.js"},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)

			// owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			// assert.Nil(t, err)

			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			// direct run.
			(func() {
				engine := NewV8Engine(ctx)
				_, err = engine.RunScriptSource(string(data), 0)
				assert.Equal(t, ErrExecutionTimeout, err)
				engine.Dispose()
			})()

			// modularized run.
			(func() {
				moduleID := fmt.Sprintf("%s", ContractName)
				runnableSource := fmt.Sprintf("require(\"%s\");", moduleID)

				engine := NewV8Engine(ctx)
				engine.AddModule(moduleID, string(data), 0)
				_, err = engine.RunScriptSource(runnableSource, 0)
				assert.Equal(t, ErrExecutionTimeout, err)
				engine.Dispose()
			})()
		})
	}
}

func TestDeployAndInitAndCall(t *testing.T) {
	tests := []struct {
		name         string
		contractPath string
		sourceType   string
		initArgs     string
		verifyArgs   string
	}{
		{"deploy sample_contract.js", "./test/sample_contract.js", "js", "[\"TEST001\", 123,[{\"name\":\"robin\",\"count\":2},{\"name\":\"roy\",\"count\":3},{\"name\":\"leon\",\"count\":4}]]", "[\"TEST001\", 123,[{\"name\":\"robin\",\"count\":2},{\"name\":\"roy\",\"count\":3},{\"name\":\"leon\",\"count\":4}]]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			assert.Nil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "dump", "")
			assert.Nil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "verify", tt.verifyArgs)
			assert.Nil(t, err)
			engine.Dispose()

			// force error.
			mem, _ = storage.NewMemoryStorage()
			context, _ = state.NewWorldState(dpos.NewDpos(), mem)
			owner, err = context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			contract, err = context.CreateContractAccount([]byte("account2"), nil)
			assert.Nil(t, err)

			ctx, err = NewContext(mockBlock(), mockTransaction(), contract, context)
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "verify", tt.verifyArgs)
			assert.NotNil(t, err)
			engine.Dispose()
		})
	}
}

func TestERC20(t *testing.T) {
	tests := []struct {
		name         string
		contractPath string
		sourceType   string
		initArgs     string
		totalSupply  string
	}{
		{"deploy ERC20.js", "./test/ERC20.js", "js", "[\"TEST001\", \"TEST\", 1000000000]", "1000000000"},
	}

	// TODO: Addd more test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			assert.Nil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "totalSupply", "[]")
			assert.Nil(t, err)
			engine.Dispose()

		})
	}
}

func TestContracts(t *testing.T) {
	type fields struct {
		function string
		args     string
	}
	tests := []struct {
		contract   string
		sourceType string
		initArgs   string
		calls      []fields
	}{
		{
			"./test/contract_rectangle.js",
			"js",
			"[\"1024\", \"768\"]",
			[]fields{
				{"calcArea", "[]"},
				{"verify", "[\"786432\"]"},
			},
		},
		{
			"./test/contract_rectangle.js",
			"js",
			"[\"999\", \"123\"]",
			[]fields{
				{"calcArea", "[]"},
				{"verify", "[\"122877\"]"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.contract, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contract)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			contract, err := context.CreateContractAccount([]byte("account2"), nil)
			assert.Nil(t, err)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			// deploy and init.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(1000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			assert.Nil(t, err)
			engine.Dispose()

			// call.
			for _, fields := range tt.calls {
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(1000, 10000000)
				_, err = engine.Call(string(data), tt.sourceType, fields.function, fields.args)
				assert.Nil(t, err)
				engine.Dispose()
			}
		})
	}
}

func TestFunctionNameCheck(t *testing.T) {
	tests := []struct {
		function    string
		expectedErr error
		args        string
	}{
		{"$dump", nil, ""},
		{"dump", nil, ""},
		{"dump_1", nil, ""},
		{"init", ErrDisallowCallPrivateFunction, ""},
		{"Init", ErrDisallowCallPrivateFunction, ""},
		{"9dump", ErrDisallowCallNotStandardFunction, ""},
		{"_dump", ErrDisallowCallNotStandardFunction, ""},
	}

	for _, tt := range tests {
		t.Run(tt.function, func(t *testing.T) {
			data, err := ioutil.ReadFile("test/sample_contract.js")
			sourceType := "js"
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(1000, 10000000)
			_, err = engine.Call(string(data), sourceType, tt.function, tt.args)
			assert.Equal(t, tt.expectedErr, err)
			engine.Dispose()
		})
	}
}
func TestMultiEngine(t *testing.T) {
	mem, _ := storage.NewMemoryStorage()
	context, _ := state.NewWorldState(dpos.NewDpos(), mem)
	owner, err := context.GetOrCreateUserAccount([]byte("account1"))
	assert.Nil(t, err)
	owner.AddBalance(newUint128FromIntWrapper(1000000))
	contract, _ := context.CreateContractAccount([]byte("account2"), nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(1000, 10000000)
			defer engine.Dispose()

			_, err = engine.RunScriptSource("console.log('running.');", 0)
			assert.Nil(t, err)
		}()
	}
	wg.Wait()
}
func TestInstructionCounterTestSuite(t *testing.T) {
	tests := []struct {
		filepath                                string
		strictDisallowUsageOfInstructionCounter int
		expectedErr                             error
		expectedResult                          string
	}{
		{"./test/instruction_counter_tests/redefine1.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine2.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine3.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine4.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine5.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine6.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine7.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/function.js", 1, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine1.js", 0, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine2.js", 0, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine3.js", 0, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine4.js", 0, core.ErrExecutionFailed, "Error: still not break the jail of _instruction_counter."},
		{"./test/instruction_counter_tests/redefine5.js", 0, ErrInjectTracingInstructionFailed, ""},
		{"./test/instruction_counter_tests/redefine6.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/redefine7.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/function.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/if.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/switch.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/for.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/with.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/while.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/throw.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/switch.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/condition_operator.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/storage_usage.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/event_usage.js", 0, nil, "\"\""},
		{"./test/instruction_counter_tests/blockchain_usage.js", 0, nil, "\"\""},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			addr, err := core.NewContractAddressFromData([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"), byteutils.FromUint64(1))
			assert.Nil(t, err)
			contract, err := context.CreateContractAccount(addr.Bytes(), nil)
			assert.Nil(t, err)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			moduleID := ContractName
			runnableSource := fmt.Sprintf("var x = require(\"%s\");", moduleID)

			engine := NewV8Engine(ctx)
			engine.strictDisallowUsageOfInstructionCounter = tt.strictDisallowUsageOfInstructionCounter
			engine.enableLimits = true
			err = engine.AddModule(moduleID, string(data), 0)
			if err != nil {
				assert.Equal(t, tt.expectedErr, err)
			} else {
				result, err := engine.RunScriptSource(runnableSource, 0)
				assert.Equal(t, tt.expectedErr, err)
				assert.Equal(t, tt.expectedResult, result)
			}
			engine.Dispose()
		})
	}
}

func TestTypeScriptExecution(t *testing.T) {
	tests := []struct {
		filepath    string
		expectedErr error
	}{
		{"./test/test_greeter.ts", nil},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			contract, err := context.CreateContractAccount([]byte("account2"), nil)
			assert.Nil(t, err)
			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

			moduleID := ContractName
			runnableSource := fmt.Sprintf("require(\"%s\");", moduleID)

			engine := NewV8Engine(ctx)
			defer engine.Dispose()

			engine.enableLimits = true
			jsSource, _, err := engine.TranspileTypeScript(string(data))
			if err != nil {
				assert.Equal(t, tt.expectedErr, err)
				return
			}

			err = engine.AddModule(moduleID, string(jsSource), 0)
			if err != nil {
				assert.Equal(t, tt.expectedErr, err)
			} else {
				_, err := engine.RunScriptSource(runnableSource, 0)
				assert.Equal(t, tt.expectedErr, err)
			}
		})
	}
}

func DeprecatedTestRunMozillaJSTestSuite(t *testing.T) {
	mem, _ := storage.NewMemoryStorage()
	context, _ := state.NewWorldState(dpos.NewDpos(), mem)
	owner, err := context.GetOrCreateUserAccount([]byte("account1"))
	assert.Nil(t, err)
	owner.AddBalance(newUint128FromIntWrapper(1000000000))

	contract, err := context.CreateContractAccount([]byte("account2"), nil)
	assert.Nil(t, err)
	ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

	var runTest func(dir string, shelljs string)
	runTest = func(dir string, shelljs string) {
		files, err := ioutil.ReadDir(dir)
		require.Nil(t, err)

		cwdShelljs := fmt.Sprintf("%s/shell.js", dir)
		if _, err := os.Stat(cwdShelljs); !os.IsNotExist(err) {
			shelljs = fmt.Sprintf("%s;%s", shelljs, cwdShelljs)
		}

		for _, file := range files {
			filepath := fmt.Sprintf("%s/%s", dir, file.Name())
			fi, err := os.Stat(filepath)
			require.Nil(t, err)

			if fi.IsDir() {
				runTest(filepath, shelljs)
				continue
			}

			if !strings.HasSuffix(file.Name(), ".js") {
				continue
			}
			if strings.Compare(file.Name(), "browser.js") == 0 || strings.Compare(file.Name(), "shell.js") == 0 || strings.HasPrefix(file.Name(), "toLocale") {
				continue
			}

			buf := bytes.NewBufferString("this.print = console.log;var native_eval = eval;eval = function (s) { try {  return native_eval(s); } catch (e) { return \"error\"; }};")

			jsfiles := fmt.Sprintf("%s;%s;%s", shelljs, "test/mozilla_js_tests_loader.js", filepath)

			for _, v := range strings.Split(jsfiles, ";") {
				if len(v) == 0 {
					continue
				}

				fi, err := os.Stat(v)
				require.Nil(t, err)
				f, err := os.Open(v)
				require.Nil(t, err)
				reader := bufio.NewReader(f)
				buf.Grow(int(fi.Size()))
				buf.ReadFrom(reader)
			}
			// execute.
			engine := NewV8Engine(ctx)
			engine.SetTestingFlag(true)
			engine.enableLimits = true
			_, err = engine.RunScriptSource(buf.String(), 0)
			//t.Logf("ret:%v, err:%v", ret, err)
			assert.Nil(t, err)
		}
	}

	runTest("test/mozilla_js_tests", "")
}

func TestBlockChain(t *testing.T) {
	tests := []struct {
		filepath    string
		expectedErr error
	}{
		{"test/test_blockchain.js", nil},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			contract, err := context.CreateContractAccount([]byte("n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR"), nil)
			assert.Nil(t, err)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(100000, 10000000)
			_, err = engine.RunScriptSource(string(data), 0)
			assert.Equal(t, tt.expectedErr, err)
			engine.Dispose()
		})
	}
}

func TestBankVaultContract(t *testing.T) {
	type TakeoutTest struct {
		args          string
		expectedErr   error
		beforeBalance string
		afterBalance  string
	}

	tests := []struct {
		name         string
		contractPath string
		sourceType   string
		saveValue    string
		saveArgs     string
		takeoutTests []TakeoutTest
	}{
		{"deploy bank_vault_contract.js", "./test/bank_vault_contract.js", "js", "5", "[0]",
			[]TakeoutTest{
				{"[1]", nil, "5", "4"},
				{"[5]", core.ErrExecutionFailed, "4", "4"},
				{"[4]", nil, "4", "0"},
				{"[1]", core.ErrExecutionFailed, "0", "0"},
			},
		},
		{"deploy bank_vault_contract.ts", "./test/bank_vault_contract.ts", "ts", "5", "[0]",
			[]TakeoutTest{
				{"[1]", nil, "5", "4"},
				{"[5]", core.ErrExecutionFailed, "4", "4"},
				{"[4]", nil, "4", "0"},
				{"[1]", core.ErrExecutionFailed, "0", "0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))

			// prepare the contract.
			addr, err := core.NewContractAddressFromData([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"), byteutils.FromUint64(1))
			assert.Nil(t, err)
			contract, _ := context.CreateContractAccount(addr.Bytes(), nil)
			contract.AddBalance(newUint128FromIntWrapper(5))

			// parepare env, block & transactions.
			tx := mockNormalTransaction("n1FkntVUMPAsESuCAAPK711omQk19JotBjM", "n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR", tt.saveValue)
			ctx, err := NewContext(mockBlock(), tx, contract, context)

			// execute.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, "")
			assert.Nil(t, err)
			engine.Dispose()

			// call save.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			_, err = engine.Call(string(data), tt.sourceType, "save", tt.saveArgs)
			assert.Nil(t, err)
			engine.Dispose()

			var (
				bal struct {
					Balance string `json:"balance"`
				}
			)

			// call takeout.
			for _, tot := range tt.takeoutTests {
				// call balanceOf.
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				balance, err := engine.Call(string(data), tt.sourceType, "balanceOf", "")
				assert.Nil(t, err)
				bal.Balance = ""
				err = json.Unmarshal([]byte(balance), &bal)
				assert.Nil(t, err)
				assert.Equal(t, tot.beforeBalance, bal.Balance)
				engine.Dispose()

				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				_, err = engine.Call(string(data), tt.sourceType, "takeout", tot.args)
				assert.Equal(t, err, tot.expectedErr)
				engine.Dispose()

				// call balanceOf.
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				balance, err = engine.Call(string(data), tt.sourceType, "balanceOf", "")
				assert.Nil(t, err)
				bal.Balance = ""
				err = json.Unmarshal([]byte(balance), &bal)
				assert.Nil(t, err)
				assert.Equal(t, tot.afterBalance, bal.Balance)
				engine.Dispose()
			}
		})
	}
}

func TestEvent(t *testing.T) {
	tests := []struct {
		filepath string
	}{
		{"test/test_event.js"},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(1000000000))
			contract, _ := context.CreateContractAccount([]byte("n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR"), nil)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(100000, 10000000)
			_, err = engine.RunScriptSource(string(data), 0)
			engine.Dispose()
		})
	}
}

func TestNRC20Contract(t *testing.T) {
	type TransferTest struct {
		to     string
		result bool
		value  string
	}

	tests := []struct {
		test          string
		contractPath  string
		sourceType    string
		name          string
		symbol        string
		decimals      int
		totalSupply   string
		from          string
		transferTests []TransferTest
	}{
		{"nrc20", "./test/NRC20.js", "js", "StandardToken标准代币", "ST", 18, "1000000000",
			"n1FkntVUMPAsESuCAAPK711omQk19JotBjM",
			[]TransferTest{
				{"n1FkntVUMPAsESuCAAPK711omQk19JotBjM", true, "5"},
				{"n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR", true, "10"},
				{"n1Kjom3J4KPsHKKzZ2xtt8Lc9W5pRDjeLcW", true, "15"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte(tt.from))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))

			// prepare the contract.
			contractAddr, err := core.AddressParse(contractStr)
			contract, _ := context.CreateContractAccount(contractAddr.Bytes(), nil)
			contract.AddBalance(newUint128FromIntWrapper(5))

			// parepare env, block & transactions.
			tx := mockNormalTransaction(tt.from, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
			ctx, err := NewContext(mockBlock(), tx, contract, context)

			// execute.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			args := fmt.Sprintf("[\"%s\", \"%s\", %d, \"%s\"]", tt.name, tt.symbol, tt.decimals, tt.totalSupply)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, args)
			assert.Nil(t, err)
			engine.Dispose()

			// call name.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			name, err := engine.Call(string(data), tt.sourceType, "name", "")
			assert.Nil(t, err)
			var nameStr string
			err = json.Unmarshal([]byte(name), &nameStr)
			assert.Nil(t, err)
			assert.Equal(t, tt.name, nameStr)
			engine.Dispose()

			// call symbol.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			symbol, err := engine.Call(string(data), tt.sourceType, "symbol", "")
			assert.Nil(t, err)
			var symbolStr string
			err = json.Unmarshal([]byte(symbol), &symbolStr)
			assert.Nil(t, err)
			assert.Equal(t, tt.symbol, symbolStr)
			assert.Nil(t, err)
			engine.Dispose()

			// call decimals.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			decimals, err := engine.Call(string(data), tt.sourceType, "decimals", "")
			assert.Nil(t, err)
			var decimalsInt int
			err = json.Unmarshal([]byte(decimals), &decimalsInt)
			assert.Nil(t, err)
			assert.Equal(t, tt.decimals, decimalsInt)
			assert.Nil(t, err)
			engine.Dispose()

			// call totalSupply.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			totalSupply, err := engine.Call(string(data), tt.sourceType, "totalSupply", "")
			assert.Nil(t, err)
			var totalSupplyStr string
			err = json.Unmarshal([]byte(totalSupply), &totalSupplyStr)
			assert.Nil(t, err)
			expect, _ := big.NewInt(0).SetString(tt.totalSupply, 10)
			expect = expect.Mul(expect, big.NewInt(0).Exp(big.NewInt(10), big.NewInt(int64(tt.decimals)), nil))
			assert.Equal(t, expect.String(), totalSupplyStr)
			assert.Nil(t, err)
			engine.Dispose()

			// call takeout.
			for _, tot := range tt.transferTests {
				// call balanceOf.
				ctx.tx = mockNormalTransaction(tt.from, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				balArgs := fmt.Sprintf("[\"%s\"]", tt.from)
				_, err := engine.Call(string(data), tt.sourceType, "balanceOf", balArgs)
				assert.Nil(t, err)
				engine.Dispose()

				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				transferArgs := fmt.Sprintf("[\"%s\", \"%s\"]", tot.to, tot.value)
				result, err := engine.Call(string(data), tt.sourceType, "transfer", transferArgs)
				assert.Nil(t, err)
				assert.Equal(t, "\"\"", result)
				engine.Dispose()

				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				approveArgs := fmt.Sprintf("[\"%s\", \"0\", \"%s\"]", tot.to, tot.value)
				result, err = engine.Call(string(data), tt.sourceType, "approve", approveArgs)
				assert.Nil(t, err)
				assert.Equal(t, "\"\"", result)
				engine.Dispose()

				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				allowanceArgs := fmt.Sprintf("[\"%s\", \"%s\"]", tt.from, tot.to)
				amount, err := engine.Call(string(data), tt.sourceType, "allowance", allowanceArgs)
				assert.Nil(t, err)
				var amountStr string
				err = json.Unmarshal([]byte(amount), &amountStr)
				assert.Nil(t, err)
				assert.Equal(t, tot.value, amountStr)
				engine.Dispose()

				ctx.tx = mockNormalTransaction(tot.to, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				transferFromArgs := fmt.Sprintf("[\"%s\", \"%s\", \"%s\"]", tt.from, tot.to, tot.value)
				result, err = engine.Call(string(data), tt.sourceType, "transferFrom", transferFromArgs)
				assert.Nil(t, err)
				assert.Equal(t, "\"\"", result)
				engine.Dispose()

				ctx.tx = mockNormalTransaction(tot.to, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 100000000)
				transferFromArgs = fmt.Sprintf("[\"%s\", \"%s\", \"%s\"]", tt.from, tot.to, tot.value)
				_, err = engine.Call(string(data), tt.sourceType, "transferFrom", transferFromArgs)
				assert.NotNil(t, err)
				engine.Dispose()
			}
		})
	}
}

func TestNRC20ContractMultitimes(t *testing.T) {
	for i := 0; i < 5; i++ {
		TestNRC20Contract(t)
	}
}

func TestNRC721Contract(t *testing.T) {

	tests := []struct {
		name         string
		contractPath string
		sourceType   string
		from         string
		to           string
		tokenID      string
	}{
		{"nrc721", "./test/NRC721BasicToken.js", "js",
			"n1FkntVUMPAsESuCAAPK711omQk19JotBjM", "n1Kjom3J4KPsHKKzZ2xtt8Lc9W5pRDjeLcW", "1001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			assert.Nil(t, err)

			// prepare the contract.
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)
			contract.AddBalance(newUint128FromIntWrapper(5))

			// parepare env, block & transactions.
			tx := mockNormalTransaction(tt.from, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
			ctx, err := NewContext(mockBlock(), tx, contract, context)

			// execute.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			args := fmt.Sprintf("[\"%s\"]", tt.name)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, args)
			assert.Nil(t, err)
			engine.Dispose()

			// call name.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			name, err := engine.Call(string(data), tt.sourceType, "name", "")
			assert.Nil(t, err)
			var nameStr string
			err = json.Unmarshal([]byte(name), &nameStr)
			assert.Nil(t, err)
			assert.Equal(t, tt.name, nameStr)
			engine.Dispose()

			// call mint.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			mintArgs := fmt.Sprintf("[\"%s\", \"%s\"]", tt.from, tt.tokenID)
			result, err := engine.Call(string(data), tt.sourceType, "mint", mintArgs)
			assert.Nil(t, err)
			assert.Equal(t, "\"\"", result)
			engine.Dispose()

			// call approve.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			approveArgs := fmt.Sprintf("[\"%s\", \"%s\"]", tt.to, tt.tokenID)
			result, err = engine.Call(string(data), tt.sourceType, "approve", approveArgs)
			assert.Nil(t, err)
			assert.Equal(t, "\"\"", result)
			engine.Dispose()

			// parepare env, block & transactions.
			tx = mockNormalTransaction(tt.to, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
			ctx, err = NewContext(mockBlock(), tx, contract, context)

			// call transferFrom.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			transferFromArgs := fmt.Sprintf("[\"%s\", \"%s\", \"%s\"]", tt.from, tt.to, tt.tokenID)
			result, err = engine.Call(string(data), tt.sourceType, "transferFrom", transferFromArgs)
			assert.Nil(t, err)
			assert.Equal(t, "\"\"", result)
			engine.Dispose()

		})
	}
}

func TestNebulasContract(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		function string
		args     string
		err      error
	}{
		{"1", "0", "unpayable", "", nil},
		{"2", "0", "unpayable", "[1]", nil},
		{"3", "1", "unpayable", "", nil},
		{"4", "0", "payable", "", core.ErrExecutionFailed},
		{"5", "1", "payable", "", nil},
		{"6", "1", "payable", "[1]", nil},
		{"7", "0", "contract1", "[1]", nil},
		{"8", "1", "contract1", "[1]", nil},
		{"9", "0", "contract2", "[1]", core.ErrExecutionFailed},
		{"10", "1", "contract2", "[1]", core.ErrExecutionFailed},
		{"11", "0", "contract3", "[1]", core.ErrExecutionFailed},
		{"12", "1", "contract3", "[1]", nil},
		{"13", "0", "contract4", "[1]", core.ErrExecutionFailed},
		{"14", "1", "contract4", "[1]", core.ErrExecutionFailed},
	}

	mem, _ := storage.NewMemoryStorage()
	context, _ := state.NewWorldState(dpos.NewDpos(), mem)

	addr, _ := core.NewAddressFromPublicKey([]byte{
		2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7,
		2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7,
		2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 2, 3, 5, 7, 1, 2, 4, 5, 3})
	owner, err := context.GetOrCreateUserAccount(addr.Bytes())
	assert.Nil(t, err)
	owner.AddBalance(newUint128FromIntWrapper(1000000000))

	addr, _ = core.NewContractAddressFromData([]byte{1, 2, 3, 5, 7}, []byte{1, 2, 3, 5, 7})
	contract, _ := context.CreateContractAccount(addr.Bytes(), nil)

	ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)

	data, err := ioutil.ReadFile("test/mixin.js")
	assert.Nil(t, err, "filepath read error")
	sourceType := "js"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx.tx = mockNormalTransaction("n1FkntVUMPAsESuCAAPK711omQk19JotBjM", "n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR", tt.value)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			_, err := engine.Call(string(data), sourceType, tt.function, tt.args)
			assert.Equal(t, tt.err, err)
			engine.Dispose()
		})
	}
}
func TestTransferValueFromContracts(t *testing.T) {
	type fields struct {
		function string
		args     string
	}
	tests := []struct {
		contract   string
		sourceType string
		initArgs   string
		calls      []fields
		value      string
		success    bool
	}{
		{
			"./test/transfer_value_from_contract.js",
			"js",
			"",
			[]fields{
				{"transfer", "[\"n1SAeQRVn33bamxN4ehWUT7JGdxipwn8b17\"]"},
			},
			"100",
			true,
		},
		{
			"./test/transfer_value_from_contract.js",
			"js",
			"",
			[]fields{
				{"transfer", "[\"n1SAeQRVn33bamxN4ehWUT7JGdxipwn8b17\"]"},
			},
			"101",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.contract, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contract)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)

			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			addr, err := core.NewContractAddressFromData([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"), byteutils.FromUint64(1))
			assert.Nil(t, err)
			contract, err := context.CreateContractAccount(addr.Bytes(), nil)
			assert.Nil(t, err)

			contract.AddBalance(newUint128FromIntWrapper(100))
			mockTx := mockNormalTransaction("n1FkntVUMPAsESuCAAPK711omQk19JotBjM", "n1FkntVUMPAsESuCAAPK711omQk19JotBjM", tt.value)
			ctx, err := NewContext(mockBlock(), mockTx, contract, context)

			// deploy and init.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(1000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			assert.Nil(t, err)
			engine.Dispose()

			// call.
			for _, fields := range tt.calls {
				engine = NewV8Engine(ctx)
				engine.SetExecutionLimits(10000, 10000000)
				result, err := engine.Call(string(data), tt.sourceType, fields.function, fields.args)
				if tt.success {
					assert.Equal(t, result, "\""+fmt.Sprint(tt.value)+"\"")
					assert.Nil(t, err)
				} else {
					assert.NotNil(t, err)
				}
				engine.Dispose()
			}
		})
	}
}

func TestRequireModule(t *testing.T) {
	tests := []struct {
		name         string
		contractPath string
		sourceType   string
		initArgs     string
	}{
		{"deploy test_require_module.js", "./test/test_require_module.js", "js", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte("account1"))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))
			contract, _ := context.CreateContractAccount([]byte("account2"), nil)

			ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, tt.initArgs)
			assert.Nil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "requireNULL", "")
			assert.NotNil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "requireNotExistPath", "")
			assert.NotNil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "requireCurPath", "")
			assert.NotNil(t, err)
			engine.Dispose()

			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 10000000)
			_, err = engine.Call(string(data), tt.sourceType, "requireNotExistFile", "")
			assert.NotNil(t, err)
			engine.Dispose()
		})
	}
}

type Neb struct {
	config    *nebletpb.Config
	chain     *core.BlockChain
	ns        net.Service
	am        *account.Manager
	genesis   *corepb.Genesis
	storage   storage.Storage
	consensus core.Consensus
	emitter   *core.EventEmitter
	nvm       core.NVM
}

func mockNeb(t *testing.T) *Neb {
	// storage, _ := storage.NewDiskStorage("test.db")
	// storage, err := storage.NewRocksStorage("rocks.db")
	// assert.Nil(t, err)
	storage, _ := storage.NewMemoryStorage()
	eventEmitter := core.NewEventEmitter(1024)
	genesisConf := MockGenesisConf()
	consensus := dpos.NewDpos()
	nvm := NewNebulasVM()
	neb := &Neb{
		genesis:   genesisConf,
		storage:   storage,
		emitter:   eventEmitter,
		consensus: consensus,
		nvm:       nvm,
		config: &nebletpb.Config{
			Chain: &nebletpb.ChainConfig{
				ChainId:    genesisConf.Meta.ChainId,
				Keydir:     "keydir",
				StartMine:  true,
				Coinbase:   "n1dYu2BXgV3xgUh8LhZu8QDDNr15tz4hVDv",
				Miner:      "n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE",
				Passphrase: "passphrase",
			},
		},
		ns: mockNetService{},
	}

	am, _ := account.NewManager(neb)
	neb.am = am

	chain, err := core.NewBlockChain(neb)
	assert.Nil(t, err)
	neb.chain = chain
	assert.Nil(t, consensus.Setup(neb))
	assert.Nil(t, chain.Setup(neb))

	var ns mockNetService
	neb.ns = ns
	neb.chain.BlockPool().RegisterInNetwork(ns)

	eventEmitter.Start()
	return neb
}

func (n *Neb) Config() *nebletpb.Config {
	return n.config
}

func (n *Neb) BlockChain() *core.BlockChain {
	return n.chain
}

func (n *Neb) NetService() net.Service {
	return n.ns
}

func (n *Neb) IsActiveSyncing() bool {
	return true
}

func (n *Neb) AccountManager() core.AccountManager {
	return n.am
}

func (n *Neb) Genesis() *corepb.Genesis {
	return n.genesis
}

func (n *Neb) Storage() storage.Storage {
	return n.storage
}

func (n *Neb) EventEmitter() *core.EventEmitter {
	return n.emitter
}

func (n *Neb) Consensus() core.Consensus {
	return n.consensus
}

func (n *Neb) Nvm() core.NVM {
	return n.nvm
}

func (n *Neb) StartActiveSync() {}

func (n *Neb) StartPprof(string) error { return nil }

func (n *Neb) SetGenesis(genesis *corepb.Genesis) {
	n.genesis = genesis
}

var (
	DefaultOpenDynasty = []string{
		"n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE",
		"n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s",
		"n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so",
		"n1JAy4X6KKLCNiTd7MWMRsVBjgdVq5WCCpf",
		"n1LkDi2gGMqPrjYcczUiweyP4RxTB6Go1qS",
		"n1LmP9K8pFF33fgdgHZonFEMsqZinJ4EUqk",
		"n1MNXBKm6uJ5d76nJTdRvkPNVq85n6CnXAi",
		"n1NrMKTYESZRCwPFDLFKiKREzZKaN1nhQvz",
		"n1NwoSCDFwFL2981k6j9DPooigW33hjAgTa",
		"n1PfACnkcfJoNm1Pbuz55pQCwueW1BYs83m",
		"n1Q8mxXp4PtHaXtebhY12BnHEwu4mryEkXH",
		"n1RYagU8n3JSuV4R7q4Qs5gQJ3pEmrZd6cJ",
		"n1SAQy3ix1pZj8MPzNeVqpAmu1nCVqb5w8c",
		"n1SHufJdxt2vRWGKAxwPETYfEq3MCQXnEXE",
		"n1SSda41zGr9FKF5DJNE2ryY1ToNrndMauN",
		"n1TmQtaCn3PNpk4f4ycwrBxCZFSVKvwBtzc",
		"n1UM7z6MqnGyKEPvUpwrfxZpM1eB7UpzmLJ",
		"n1UnCsJZjQiKyQiPBr7qG27exqCLuWUf1d7",
		"n1XkoVVjswb5Gek3rRufqjKNpwrDdsnQ7Hq",
		"n1cYKNHTeVW9v1NQRWuhZZn9ETbqAYozckh",
		"n1dYu2BXgV3xgUh8LhZu8QDDNr15tz4hVDv",
	}
)

// MockGenesisConf return mock genesis conf
func MockGenesisConf() *corepb.Genesis {
	dynasty := []string{}
	for _, v := range DefaultOpenDynasty {
		dynasty = append(dynasty, v)
	}
	return &corepb.Genesis{
		Meta: &corepb.GenesisMeta{ChainId: 0},
		Consensus: &corepb.GenesisConsensus{
			Dpos: &corepb.GenesisConsensusDpos{
				Dynasty: dynasty,
			},
		},
		TokenDistribution: []*corepb.GenesisTokenDistribution{
			&corepb.GenesisTokenDistribution{
				Address: "n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE",
				Value:   "5000000000000000000000000",
			},
			&corepb.GenesisTokenDistribution{
				Address: "n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s",
				Value:   "5000000000000000000000000",
			},
			&corepb.GenesisTokenDistribution{
				Address: "n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so",
				Value:   "5000000000000000000000000",
			},
			&corepb.GenesisTokenDistribution{
				Address: "n1JAy4X6KKLCNiTd7MWMRsVBjgdVq5WCCpf",
				Value:   "5000000000000000000000000",
			},
			&corepb.GenesisTokenDistribution{
				Address: "n1LkDi2gGMqPrjYcczUiweyP4RxTB6Go1qS",
				Value:   "5000000000000000000000000",
			},
			&corepb.GenesisTokenDistribution{
				Address: "n1LmP9K8pFF33fgdgHZonFEMsqZinJ4EUqk",
				Value:   "5000000000000000000000000",
			},
		},
	}
}

var (
	received = []byte{}
)

type mockNetService struct{}

func (n mockNetService) Start() error { return nil }
func (n mockNetService) Stop()        {}

func (n mockNetService) Node() *net.Node { return nil }

func (n mockNetService) Sync(net.Serializable) error { return nil }

func (n mockNetService) Register(...*net.Subscriber)   {}
func (n mockNetService) Deregister(...*net.Subscriber) {}

func (n mockNetService) Broadcast(name string, msg net.Serializable, priority int) {
	pb, _ := msg.ToProto()
	bytes, _ := proto.Marshal(pb)
	received = bytes
}
func (n mockNetService) Relay(name string, msg net.Serializable, priority int) {
	pb, _ := msg.ToProto()
	bytes, _ := proto.Marshal(pb)
	received = bytes
}
func (n mockNetService) SendMsg(name string, msg []byte, target string, priority int) error {
	received = msg
	return nil
}

func (n mockNetService) SendMessageToPeers(messageName string, data []byte, priority int, filter net.PeerFilterAlgorithm) []string {
	return make([]string, 0)
}
func (n mockNetService) SendMessageToPeer(messageName string, data []byte, priority int, peerID string) error {
	return nil
}

func (n mockNetService) ClosePeer(peerID string, reason error) {}

func (n mockNetService) BroadcastNetworkID([]byte) {}

type contract struct {
	contractPath string
	sourceType   string
	initArgs     string
}

type call struct {
	function   string
	args       string
	exceptArgs []string //[congractA,B,C, AccountA, B]
}

func TestInnerTransactions(t *testing.T) {
	tests := []struct {
		name      string
		contracts []contract
		calls     []call
	}{
		{
			"deploy test_require_module.js",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract.js",
					"js",
					"",
				},
			},
			[]call{
				call{
					"save",
					"[1]",
					[]string{"1", "3", "2", "4999999999999905351999994", "5000001426940068783000000"},
				},
			},
		},
	}
	tt := tests[0]
	for _, call := range tt.calls {

		neb := mockNeb(t)
		tail := neb.chain.TailBlock()
		manager, err := account.NewManager(neb)
		assert.Nil(t, err)

		a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
		assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
		b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
		assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
		c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
		assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

		elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
		consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
		assert.Nil(t, err)
		block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
		assert.Nil(t, err)
		block.WorldState().SetConsensusState(consensusState)
		block.SetTimestamp(consensusState.TimeStamp())

		contractsAddr := []string{}

		// t.Run(tt.name, func(t *testing.T) {
		for k, v := range tt.contracts {
			data, err := ioutil.ReadFile(v.contractPath)
			assert.Nil(t, err, "contract path read error")
			source := string(data)
			sourceType := "js"
			argsDeploy := ""
			deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
			payloadDeploy, _ := deploy.ToBytes()

			value, _ := util.NewUint128FromInt(0)
			gasLimit, _ := util.NewUint128FromInt(200000)
			txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txDeploy))
			assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

			contractAddr, err := txDeploy.GenerateContractAddress()
			assert.Nil(t, err)
			contractsAddr = append(contractsAddr, contractAddr.String())
		}
		// })

		block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
		assert.Nil(t, block.Seal())
		assert.Nil(t, manager.SignBlock(b, block))
		assert.Nil(t, neb.chain.BlockPool().Push(block))

		for _, v := range contractsAddr {
			contract, err := core.AddressParse(v)
			assert.Nil(t, err)
			_, err = neb.chain.TailBlock().CheckContract(contract)
			assert.Nil(t, err)
		}

		elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
		tail = neb.chain.TailBlock()
		consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
		assert.Nil(t, err)
		block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
		assert.Nil(t, err)
		block.WorldState().SetConsensusState(consensusState)
		block.SetTimestamp(consensusState.TimeStamp())
		//accountA, err := tail.GetAccount(a.Bytes())
		//accountB, err := tail.GetAccount(b.Bytes())
		assert.Nil(t, err)

		calleeContract := contractsAddr[1]
		callToContract := contractsAddr[2]
		callPayload, _ := core.NewCallPayload(call.function, fmt.Sprintf("[\"%s\", \"%s\", 1]", calleeContract, callToContract))
		payloadCall, _ := callPayload.ToBytes()

		value, _ := util.NewUint128FromInt(6)
		gasLimit, _ := util.NewUint128FromInt(200000)

		proxyContractAddress, err := core.AddressParse(contractsAddr[0])
		txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
			uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
		assert.Nil(t, err)
		assert.Nil(t, manager.SignTransaction(a, txCall))
		assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

		block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
		assert.Nil(t, block.Seal())
		assert.Nil(t, manager.SignBlock(c, block))
		assert.Nil(t, neb.chain.BlockPool().Push(block))

		// check
		tail = neb.chain.TailBlock()
		// event, err := tail.FetchExecutionResultEvent(txCall.Hash())
		// assert.Nil(t, err)
		// txEvent := core.TransactionEvent{}
		// err = json.Unmarshal([]byte(event.Data), &txEvent)
		// assert.Nil(t, err)
		// // if txEvent.Status != 1 {
		// // 	fmt.Println(txEvent)
		// // }
		// fmt.Println("=====================", txEvent)

		events, err := tail.FetchEvents(txCall.Hash())
		assert.Nil(t, err)
		for _, event := range events {

			fmt.Println("==============", event.Data)
		}
		contractAddrA, err := core.AddressParse(contractsAddr[0])
		accountAAcc, err := tail.GetAccount(contractAddrA.Bytes())
		assert.Nil(t, err)
		fmt.Printf("account :%v\n", accountAAcc)
		assert.Equal(t, call.exceptArgs[0], accountAAcc.Balance().String())

		contractAddrB, err := core.AddressParse(contractsAddr[1])
		accountBAcc, err := tail.GetAccount(contractAddrB.Bytes())
		assert.Nil(t, err)
		fmt.Printf("accountB :%v\n", accountBAcc)
		assert.Equal(t, call.exceptArgs[1], accountBAcc.Balance().String())

		contractAddrC, err := core.AddressParse(contractsAddr[2])
		accountAccC, err := tail.GetAccount(contractAddrC.Bytes())
		assert.Nil(t, err)
		fmt.Printf("accountC :%v\n", accountAccC)
		assert.Equal(t, call.exceptArgs[2], accountAccC.Balance().String())

		aI, err := tail.GetAccount(a.Bytes())
		// assert.Equal(t, call.exceptArgs[3], aI.Balance().String())
		fmt.Printf("aI:%v\n", aI)
		bI, err := tail.GetAccount(b.Bytes())
		fmt.Printf("b:%v\n", bI)
		// assert.Equal(t, call.exceptArgs[4], bI.Balance().String())
		// assert.Equal(t, txEvent.Status, 1)
	}
}

func TestInnerTransactionsMaxMulit(t *testing.T) {
	tests := []struct {
		name        string
		contracts   []contract
		call        call
		expectedErr string
	}{
		{
			"deploy test_require_module.js",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract.js",
					"js",
					"",
				},
			},
			call{
				"saveToLoop",
				"[1]",
				[]string{""},
			},
			"multi execution failed",
		},
	}

	for _, tt := range tests {
		neb := mockNeb(t)
		tail := neb.chain.TailBlock()
		manager, err := account.NewManager(neb)
		assert.Nil(t, err)

		a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
		assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
		b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
		assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
		c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
		assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

		elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
		consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
		assert.Nil(t, err)
		block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
		assert.Nil(t, err)
		block.WorldState().SetConsensusState(consensusState)
		block.SetTimestamp(consensusState.TimeStamp())

		contractsAddr := []string{}
		fmt.Printf("++++++++++++pack account")
		// t.Run(tt.name, func(t *testing.T) {
		for k, v := range tt.contracts {
			data, err := ioutil.ReadFile(v.contractPath)
			assert.Nil(t, err, "contract path read error")
			source := string(data)
			sourceType := "js"
			argsDeploy := ""
			deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
			payloadDeploy, _ := deploy.ToBytes()

			value, _ := util.NewUint128FromInt(0)
			gasLimit, _ := util.NewUint128FromInt(200000)
			txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txDeploy))
			assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

			contractAddr, err := txDeploy.GenerateContractAddress()
			assert.Nil(t, err)
			contractsAddr = append(contractsAddr, contractAddr.String())
		}
		// })

		block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
		assert.Nil(t, block.Seal())
		assert.Nil(t, manager.SignBlock(b, block))
		assert.Nil(t, neb.chain.BlockPool().Push(block))

		for _, v := range contractsAddr {
			contract, err := core.AddressParse(v)
			assert.Nil(t, err)
			_, err = neb.chain.TailBlock().CheckContract(contract)
			assert.Nil(t, err)
		}

		elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
		tail = neb.chain.TailBlock()
		consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
		assert.Nil(t, err)
		block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
		assert.Nil(t, err)
		block.WorldState().SetConsensusState(consensusState)
		block.SetTimestamp(consensusState.TimeStamp())
		//accountA, err := tail.GetAccount(a.Bytes())
		//accountB, err := tail.GetAccount(b.Bytes())
		assert.Nil(t, err)

		calleeContract := contractsAddr[0]
		callToContract := contractsAddr[2]
		fmt.Printf("++++++++++++pack payload")
		callPayload, _ := core.NewCallPayload(tt.call.function, fmt.Sprintf("[\"%s\", \"%s\", 1]", calleeContract, callToContract))
		payloadCall, _ := callPayload.ToBytes()

		value, _ := util.NewUint128FromInt(6)
		gasLimit, _ := util.NewUint128FromInt(200000)

		proxyContractAddress, err := core.AddressParse(contractsAddr[0])
		fmt.Printf("++++++++++++pack transaction")
		txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
			uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
		assert.Nil(t, err)
		assert.Nil(t, manager.SignTransaction(a, txCall))
		assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

		fmt.Printf("++++++++++++pack collect")
		block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
		assert.Nil(t, block.Seal())
		assert.Nil(t, manager.SignBlock(c, block))
		assert.Nil(t, neb.chain.BlockPool().Push(block))

		fmt.Printf("++++++++++++pack check\n")
		// check
		tail = neb.chain.TailBlock()

		events, err := tail.FetchEvents(txCall.Hash())
		assert.Nil(t, err)
		// assert.Equal(t, len(events), 1)
		// events.
		fmt.Printf("==events:%v\n", events)
		for _, event := range events {

			fmt.Println("==============", event.Data)
		}
		//
	}
}
func TestInnerTransactionsGasLimit(t *testing.T) {
	tests := []struct {
		name           string
		contracts      []contract
		call           call
		expectedErr    string
		gasArr         []int
		gasExpectedErr []string
	}{
		{
			"deploy test_require_module.js",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract.js",
					"js",
					"",
				},
			},
			call{
				"save",
				"[1]",
				[]string{""},
			},
			"multi execution failed",
			//[]int{10000, 20000, 21300, 25300, 31500},
			//25118 in c and gas is 1
			//25117 in B and after cost c is 0
			//25116 in B not enough to cost in C
			//23117 在B内不足支付到C{engine.call system failed the gas over!!!,engine index:1}
			//22436 A不足支付B{engine.call system failed the gas over!!!,engine index:0}
			//22437 A刚好支付B剩余1{engine.call insuff limit err:insufficient gas,engine index:0}
			//20336 仅够支付A{engine.call system failed the gas over!!!,engine index:0}
			//20335 A不足gas{insufficient gas}
			//10000 不能进入trans
			//tmp 23117
			[]int{25218},
			[]string{"", "",
				"engine.call system failed the gas over!!!, engine index:0",
				"engine.call insuff limit err:insufficient gas, engine index:1"},
		},
	}

	for _, tt := range tests {
		for i := 0; i < len(tt.gasArr); i++ {

			neb := mockNeb(t)
			tail := neb.chain.TailBlock()
			manager, err := account.NewManager(neb)
			assert.Nil(t, err)

			a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
			assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
			b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
			assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
			c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
			assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

			elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
			consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())

			contractsAddr := []string{}
			fmt.Printf("++++++++++++pack account")
			// t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.contracts {
				data, err := ioutil.ReadFile(v.contractPath)
				assert.Nil(t, err, "contract path read error")
				source := string(data)
				sourceType := "js"
				argsDeploy := ""
				deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
				payloadDeploy, _ := deploy.ToBytes()

				value, _ := util.NewUint128FromInt(0)
				gasLimit, _ := util.NewUint128FromInt(200000)
				txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
				assert.Nil(t, err)
				assert.Nil(t, manager.SignTransaction(a, txDeploy))
				assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

				contractAddr, err := txDeploy.GenerateContractAddress()
				assert.Nil(t, err)
				contractsAddr = append(contractsAddr, contractAddr.String())
			}
			// })

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(b, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			for _, v := range contractsAddr {
				contract, err := core.AddressParse(v)
				assert.Nil(t, err)
				_, err = neb.chain.TailBlock().CheckContract(contract)
				assert.Nil(t, err)
			}

			elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
			tail = neb.chain.TailBlock()
			consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())
			//accountA, err := tail.GetAccount(a.Bytes())
			//accountB, err := tail.GetAccount(b.Bytes())
			assert.Nil(t, err)

			calleeContract := contractsAddr[1]
			callToContract := contractsAddr[2]
			fmt.Printf("++++++++++++pack payload")
			callPayload, _ := core.NewCallPayload(tt.call.function, fmt.Sprintf("[\"%s\", \"%s\", 1]", calleeContract, callToContract))
			payloadCall, _ := callPayload.ToBytes()

			value, _ := util.NewUint128FromInt(6)
			//gasLimit, _ := util.NewUint128FromInt(21300)
			//gasLimit, _ := util.NewUint128FromInt(25300)	//null                            file=logger.go func=nvm.V8Log line=32
			gasLimit, _ := util.NewUint128FromInt(int64(tt.gasArr[i]))
			proxyContractAddress, err := core.AddressParse(contractsAddr[0])
			fmt.Printf("++++++++++++pack transaction")
			txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
				uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txCall))
			assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

			fmt.Printf("++++++++++++pack collect")
			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(c, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			fmt.Printf("++++++++++++pack check\n")
			// check
			tail = neb.chain.TailBlock()

			events, err := tail.FetchEvents(txCall.Hash())
			//assert.Nil(t, err)
			// events.
			fmt.Printf("==events:%v\n", events)
			for _, event := range events {

				fmt.Println("==============", event.Data)
			}
			/*
				contractOne, err := core.AddressParse(contractsAddr[0])
				accountANew, err := tail.GetAccount(contractOne.Bytes())
				assert.Nil(t, err)
				fmt.Printf("contractA account :%v\n", accountANew)

				contractTwo, err := core.AddressParse(contractsAddr[1])
				accountBNew, err := tail.GetAccount(contractTwo.Bytes())
				assert.Nil(t, err)
				fmt.Printf("contractB account :%v\n", accountBNew)

				aI, err := tail.GetAccount(a.Bytes())
				// bI, err := tail.GetAccount(b.Bytes())
				fmt.Printf("aI:%v\n", aI)
				bI, err := tail.GetAccount(b.Bytes())
				fmt.Printf("bI:%v\n", bI)*/
		}
		//
	}
}

type SysEvent struct {
	Hash    string `json:"hash"`
	Status  int    `json:"status"`
	GasUsed string `json:"gas_used"`
	Err     string `json:"error"`
}

func TestInnerTransactionsMemLimit(t *testing.T) {
	tests := []struct {
		name           string
		contracts      []contract
		call           call
		expectedErr    string
		memArr         []int
		memExpectedErr []string
	}{
		{
			"deploy test_require_module.js",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract.js",
					"js",
					"",
				},
			},
			call{
				"saveMem",
				"[1]",
				[]string{""},
			},
			"multi execution failed",
			[]int{5 * 1024 * 1024, 10 * 1024 * 1024, 20 * 1024 * 1024, 40 * 1024 * 1024},
			[]string{"",
				"Inner Call: inner transation err [exceed memory limits] engine index:1",
				"Inner Call: inner transation err [exceed memory limits] engine index:0",
				"exceed memory limits"},
		},
	}

	for _, tt := range tests {
		for i := 0; i < len(tt.memArr); i++ {

			neb := mockNeb(t)
			tail := neb.chain.TailBlock()
			manager, err := account.NewManager(neb)
			assert.Nil(t, err)

			a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
			assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
			b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
			assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
			c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
			assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

			elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
			consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())

			contractsAddr := []string{}
			for k, v := range tt.contracts {
				data, err := ioutil.ReadFile(v.contractPath)
				assert.Nil(t, err, "contract path read error")
				source := string(data)
				sourceType := "js"
				argsDeploy := ""
				deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
				payloadDeploy, _ := deploy.ToBytes()

				value, _ := util.NewUint128FromInt(0)
				gasLimit, _ := util.NewUint128FromInt(200000)
				txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
				assert.Nil(t, err)
				assert.Nil(t, manager.SignTransaction(a, txDeploy))
				assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

				contractAddr, err := txDeploy.GenerateContractAddress()
				assert.Nil(t, err)
				contractsAddr = append(contractsAddr, contractAddr.String())
			}

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(b, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			for _, v := range contractsAddr {
				contract, err := core.AddressParse(v)
				assert.Nil(t, err)
				_, err = neb.chain.TailBlock().CheckContract(contract)
				assert.Nil(t, err)
			}

			elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
			tail = neb.chain.TailBlock()
			consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())
			assert.Nil(t, err)

			calleeContract := contractsAddr[1]
			callToContract := contractsAddr[2]
			callPayload, _ := core.NewCallPayload(tt.call.function, fmt.Sprintf("[\"%s\", \"%s\", \"%d\"]", calleeContract, callToContract, tt.memArr[i]))
			payloadCall, _ := callPayload.ToBytes()

			value, _ := util.NewUint128FromInt(6)
			gasLimit, _ := util.NewUint128FromInt(int64(tt.memArr[i]))
			proxyContractAddress, err := core.AddressParse(contractsAddr[0])
			txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
				uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txCall))
			assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(c, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			tail = neb.chain.TailBlock()
			events, err := tail.FetchEvents(txCall.Hash())
			for _, event := range events {

				var jEvent SysEvent
				if err := json.Unmarshal([]byte(event.Data), &jEvent); err == nil {
					if jEvent.Hash != "" {
						assert.Equal(t, tt.memExpectedErr[i], jEvent.Err)
					}
				}

			}
		}
	}
}

func TestInnerTransactionsErr(t *testing.T) {
	tests := []struct {
		name           string
		contracts      []contract
		call           call
		errFlagArr     []uint32
		expectedErrArr []string
	}{
		{
			"deploy TestInnerTransactionsErr.js",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract.js",
					"js",
					"",
				},
			},
			call{
				"saveErr",
				"[1]",
				[]string{""},
			},
			[]uint32{0, 1, 2},
			[]string{"Call: saveErr in test_inner_transaction",
				"Inner Call: inner transation err [execution failed] engine index:0",
				"Inner Call: inner transation err [execution failed] engine index:1"},
		},
	}

	for _, tt := range tests {
		for i := 0; i < len(tt.errFlagArr); i++ {

			neb := mockNeb(t)
			tail := neb.chain.TailBlock()
			manager, err := account.NewManager(neb)
			assert.Nil(t, err)

			a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
			assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
			b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
			assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
			c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
			assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

			elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
			consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())

			contractsAddr := []string{}
			for k, v := range tt.contracts {
				data, err := ioutil.ReadFile(v.contractPath)
				assert.Nil(t, err, "contract path read error")
				source := string(data)
				sourceType := "js"
				argsDeploy := ""
				deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
				payloadDeploy, _ := deploy.ToBytes()

				value, _ := util.NewUint128FromInt(0)
				gasLimit, _ := util.NewUint128FromInt(200000)
				txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
				assert.Nil(t, err)
				assert.Nil(t, manager.SignTransaction(a, txDeploy))
				assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

				contractAddr, err := txDeploy.GenerateContractAddress()
				assert.Nil(t, err)
				contractsAddr = append(contractsAddr, contractAddr.String())
			}

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(b, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			for _, v := range contractsAddr {
				contract, err := core.AddressParse(v)
				assert.Nil(t, err)
				_, err = neb.chain.TailBlock().CheckContract(contract)
				assert.Nil(t, err)
			}

			elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
			tail = neb.chain.TailBlock()
			consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())
			assert.Nil(t, err)

			calleeContract := contractsAddr[1]
			callToContract := contractsAddr[2]
			callPayload, _ := core.NewCallPayload(tt.call.function, fmt.Sprintf("[\"%s\", \"%s\", \"%d\"]", calleeContract, callToContract, tt.errFlagArr[i]))
			payloadCall, _ := callPayload.ToBytes()

			value, _ := util.NewUint128FromInt(6)
			gasLimit, _ := util.NewUint128FromInt(1000000)
			proxyContractAddress, err := core.AddressParse(contractsAddr[0])
			txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
				uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txCall))
			assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(c, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			tail = neb.chain.TailBlock()
			events, err := tail.FetchEvents(txCall.Hash())
			for _, event := range events {

				var jEvent SysEvent
				if err := json.Unmarshal([]byte(event.Data), &jEvent); err == nil {
					if jEvent.Hash != "" {
						assert.Equal(t, tt.expectedErrArr[i], jEvent.Err)
					}
				}

			}
		}
	}
}

func TestThreadStackOverflow(t *testing.T) {
	tests := []struct {
		filepath    string
		expectedErr error
	}{
		{"test/test_stack_overflow.js", core.ErrExecutionFailed},
	}
	// lockx := sync.RWMutex{}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.filepath)
			assert.Nil(t, err, "filepath read error")
			for j := 0; j < 10; j++ {

				var wg sync.WaitGroup
				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						mem, _ := storage.NewMemoryStorage()
						context, _ := state.NewWorldState(dpos.NewDpos(), mem)
						owner, err := context.GetOrCreateUserAccount([]byte("n1FkntVUMPAsESuCAAPK711omQk19JotBjM"))
						assert.Nil(t, err)
						owner.AddBalance(newUint128FromIntWrapper(1000000000))
						contract, err := context.CreateContractAccount([]byte("n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR"), nil)
						assert.Nil(t, err)

						ctx, err := NewContext(mockBlock(), mockTransaction(), contract, context)
						engine := NewV8Engine(ctx)
						engine.SetExecutionLimits(100000000, 10000000)
						_, err = engine.DeployAndInit(string(data), "js", "")
						fmt.Printf("err:%v", err)
						// _, err = engine.RunScriptSource("", 0)
						assert.Equal(t, tt.expectedErr, err)
						engine.Dispose()

					}()
					// }
				}
				wg.Wait()
			}

		})
	}
}
func TestGetContractErr(t *testing.T) {
	tests := []struct {
		name      string
		contracts []contract
		calls     []call
	}{
		{
			"TestGetContractErr",
			[]contract{
				contract{
					"./test/test_inner_transaction.js",
					"js",
					"",
				},
				contract{
					"./test/bank_vault_contract_second.js",
					"js",
					"",
				},
			},
			[]call{
				call{
					"getSource",
					"[1]",
					[]string{"Call: Inner Call: no contract at this address"},
				},
			},
		},
	}

	for _, tt := range tests {
		for i := 0; i < len(tt.calls); i++ {

			neb := mockNeb(t)
			tail := neb.chain.TailBlock()
			manager, err := account.NewManager(neb)
			assert.Nil(t, err)

			a, _ := core.AddressParse("n1FF1nz6tarkDVwWQkMnnwFPuPKUaQTdptE")
			assert.Nil(t, manager.Unlock(a, []byte("passphrase"), keystore.YearUnlockDuration))
			b, _ := core.AddressParse("n1GmkKH6nBMw4rrjt16RrJ9WcgvKUtAZP1s")
			assert.Nil(t, manager.Unlock(b, []byte("passphrase"), keystore.YearUnlockDuration))
			c, _ := core.AddressParse("n1H4MYms9F55ehcvygwWE71J8tJC4CRr2so")
			assert.Nil(t, manager.Unlock(c, []byte("passphrase"), keystore.YearUnlockDuration))

			elapsedSecond := dpos.BlockIntervalInMs / dpos.SecondInMs
			consensusState, err := tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err := core.NewBlock(neb.chain.ChainID(), b, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())

			contractsAddr := []string{}
			for k, v := range tt.contracts {
				data, err := ioutil.ReadFile(v.contractPath)
				assert.Nil(t, err, "contract path read error")
				source := string(data)
				sourceType := "js"
				argsDeploy := ""
				deploy, _ := core.NewDeployPayload(source, sourceType, argsDeploy)
				payloadDeploy, _ := deploy.ToBytes()

				value, _ := util.NewUint128FromInt(0)
				gasLimit, _ := util.NewUint128FromInt(200000)
				txDeploy, err := core.NewTransaction(neb.chain.ChainID(), a, a, value, uint64(k+1), core.TxPayloadDeployType, payloadDeploy, core.TransactionGasPrice, gasLimit)
				assert.Nil(t, err)
				assert.Nil(t, manager.SignTransaction(a, txDeploy))
				assert.Nil(t, neb.chain.TransactionPool().Push(txDeploy))

				contractAddr, err := txDeploy.GenerateContractAddress()
				assert.Nil(t, err)
				contractsAddr = append(contractsAddr, contractAddr.String())
			}

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(b, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			for _, v := range contractsAddr {
				contract, err := core.AddressParse(v)
				assert.Nil(t, err)
				_, err = neb.chain.TailBlock().CheckContract(contract)
				assert.Nil(t, err)
			}

			elapsedSecond = dpos.BlockIntervalInMs / dpos.SecondInMs
			tail = neb.chain.TailBlock()
			consensusState, err = tail.WorldState().NextConsensusState(elapsedSecond)
			assert.Nil(t, err)
			block, err = core.NewBlock(neb.chain.ChainID(), c, tail)
			assert.Nil(t, err)
			block.WorldState().SetConsensusState(consensusState)
			block.SetTimestamp(consensusState.TimeStamp())
			assert.Nil(t, err)

			calleeContract := "123456789"
			callToContract := "123456789"
			callPayload, _ := core.NewCallPayload(tt.calls[i].function, fmt.Sprintf("[\"%s\", \"%s\"]", calleeContract, callToContract))
			payloadCall, _ := callPayload.ToBytes()

			value, _ := util.NewUint128FromInt(6)
			gasLimit, _ := util.NewUint128FromInt(1000000)
			proxyContractAddress, err := core.AddressParse(contractsAddr[0])
			txCall, err := core.NewTransaction(neb.chain.ChainID(), a, proxyContractAddress, value,
				uint64(len(contractsAddr)+1), core.TxPayloadCallType, payloadCall, core.TransactionGasPrice, gasLimit)
			assert.Nil(t, err)
			assert.Nil(t, manager.SignTransaction(a, txCall))
			assert.Nil(t, neb.chain.TransactionPool().Push(txCall))

			block.CollectTransactions((time.Now().Unix() + 1) * dpos.SecondInMs)
			assert.Nil(t, block.Seal())
			assert.Nil(t, manager.SignBlock(c, block))
			assert.Nil(t, neb.chain.BlockPool().Push(block))

			tail = neb.chain.TailBlock()
			events, err := tail.FetchEvents(txCall.Hash())
			for _, event := range events {
				fmt.Printf("event:%v\n", events)
				var jEvent SysEvent
				if err := json.Unmarshal([]byte(event.Data), &jEvent); err == nil {
					if jEvent.Hash != "" {
						assert.Equal(t, tt.calls[i].exceptArgs[0], jEvent.Err)
					}
					fmt.Printf("event:%v\n", jEvent.Err)
				}

			}
		}
	}
}

func TestGetRandomBySingle(t *testing.T) {
	type TransferTest struct {
		to     string
		result bool
		value  string
	}

	tests := []struct {
		test          string
		contractPath  string
		sourceType    string
		name          string
		symbol        string
		decimals      int
		totalSupply   string
		from          string
		transferTests []TransferTest
	}{
		{"getRandomBySingle", "./test/test_inner_transaction.js", "js", "StandardToken标准代币", "ST", 18, "1000000000",
			"n1FkntVUMPAsESuCAAPK711omQk19JotBjM",
			[]TransferTest{
				{"n1FkntVUMPAsESuCAAPK711omQk19JotBjM", true, "5"},
				{"n1JNHZJEUvfBYfjDRD14Q73FX62nJAzXkMR", true, "10"},
				{"n1Kjom3J4KPsHKKzZ2xtt8Lc9W5pRDjeLcW", true, "15"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ioutil.ReadFile(tt.contractPath)
			assert.Nil(t, err, "contract path read error")

			mem, _ := storage.NewMemoryStorage()
			context, _ := state.NewWorldState(dpos.NewDpos(), mem)
			owner, err := context.GetOrCreateUserAccount([]byte(tt.from))
			assert.Nil(t, err)
			owner.AddBalance(newUint128FromIntWrapper(10000000))

			// prepare the contract.
			contractAddr, err := core.AddressParse(contractStr)
			contract, _ := context.CreateContractAccount(contractAddr.Bytes(), nil)
			contract.AddBalance(newUint128FromIntWrapper(5))

			// parepare env, block & transactions.
			tx := mockNormalTransaction(tt.from, "n1TV3sU6jyzR4rJ1D7jCAmtVGSntJagXZHC", "0")
			ctx, err := NewContext(mockBlock(), tx, contract, context)

			// execute.
			engine := NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			args := fmt.Sprintf("[\"%s\", \"%s\", %d, \"%s\"]", tt.name, tt.symbol, tt.decimals, tt.totalSupply)
			_, err = engine.DeployAndInit(string(data), tt.sourceType, args)
			assert.Nil(t, err)
			engine.Dispose()

			// call name.
			engine = NewV8Engine(ctx)
			engine.SetExecutionLimits(10000, 100000000)
			rand, err := engine.Call(string(data), tt.sourceType, "getRandom", "")
			fmt.Printf("rand:%v\n", rand)
			assert.Nil(t, err)
			// var nameStr string
			// err = json.Unmarshal([]byte(name), &nameStr)
			// assert.Nil(t, err)
			// assert.Equal(t, tt.name, nameStr)
			engine.Dispose()

		})
	}
}
