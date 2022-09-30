package compiler_test

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/nspcc-dev/neo-go/internal/fakechain"
	"github.com/nspcc-dev/neo-go/pkg/compiler"
	"github.com/nspcc-dev/neo-go/pkg/core"
	"github.com/nspcc-dev/neo-go/pkg/core/dao"
	"github.com/nspcc-dev/neo-go/pkg/core/interop"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/encoding/base58"
	cinterop "github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/callflag"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestTypeConstantSize(t *testing.T) {
	src := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop"
	var a %T // type declaration is always ok
	func Main() interface{} {
		return %#v
	}`

	t.Run("Hash160", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			a := make(cinterop.Hash160, 20)
			src := fmt.Sprintf(src, a, a)
			eval(t, src, []byte(a))
		})
		t.Run("bad", func(t *testing.T) {
			a := make(cinterop.Hash160, 19)
			src := fmt.Sprintf(src, a, a)
			_, err := compiler.Compile("foo.go", strings.NewReader(src))
			require.Error(t, err)
		})
	})
	t.Run("Hash256", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			a := make(cinterop.Hash256, 32)
			src := fmt.Sprintf(src, a, a)
			eval(t, src, []byte(a))
		})
		t.Run("bad", func(t *testing.T) {
			a := make(cinterop.Hash256, 31)
			src := fmt.Sprintf(src, a, a)
			_, err := compiler.Compile("foo.go", strings.NewReader(src))
			require.Error(t, err)
		})
	})
}

func TestFromAddress(t *testing.T) {
	as1 := "NQRLhCpAru9BjGsMwk67vdMwmzKMRgsnnN"
	addr1, err := address.StringToUint160(as1)
	require.NoError(t, err)

	as2 := "NPAsqZkx9WhNd4P72uhZxBhLinSuNkxfB8"
	addr2, err := address.StringToUint160(as2)
	require.NoError(t, err)

	t.Run("append 2 addresses", func(t *testing.T) {
		src := `
		package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/util"
		func Main() []byte {
			addr1 := util.FromAddress("` + as1 + `")
			addr2 := util.FromAddress("` + as2 + `")
			sum := append(addr1, addr2...)
			return sum
		}
		`

		eval(t, src, append(addr1.BytesBE(), addr2.BytesBE()...))
	})

	t.Run("append 2 addresses inline", func(t *testing.T) {
		src := `
		package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/util"
		func Main() []byte {
			addr1 := util.FromAddress("` + as1 + `")
			sum := append(addr1, util.FromAddress("` + as2 + `")...)
			return sum
		}
		`

		eval(t, src, append(addr1.BytesBE(), addr2.BytesBE()...))
	})

	t.Run("AliasPackage", func(t *testing.T) {
		src := `
		package foo
		import uu "github.com/nspcc-dev/neo-go/pkg/interop/util"
		func Main() []byte {
			addr1 := uu.FromAddress("` + as1 + `")
			addr2 := uu.FromAddress("` + as2 + `")
			sum := append(addr1, addr2...)
			return sum
		}`
		eval(t, src, append(addr1.BytesBE(), addr2.BytesBE()...))
	})
}

func TestAddressToHash160BuiltinConversion(t *testing.T) {
	a := "NQRLhCpAru9BjGsMwk67vdMwmzKMRgsnnN"
	h, err := address.StringToUint160(a)
	require.NoError(t, err)
	t.Run("builtin conversion", func(t *testing.T) {
		src := `package foo
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop"
			"github.com/nspcc-dev/neo-go/pkg/interop/lib/address"
		)
		var addr = address.ToHash160("` + a + `")
		func Main() interop.Hash160 {
			return addr
		}`
		prog := eval(t, src, h.BytesBE())
		// Address BE bytes expected to be present at program, which indicates that address conversion
		// was performed at compile-time.
		require.True(t, strings.Contains(string(prog), string(h.BytesBE())))
		// On the contrary, there should be no address string.
		require.False(t, strings.Contains(string(prog), a))
	})
	t.Run("generate code", func(t *testing.T) {
		src := `package foo
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop"
			"github.com/nspcc-dev/neo-go/pkg/interop/lib/address"
		)
		var addr = "` + a + `"
		func Main() interop.Hash160 {
			return address.ToHash160(addr)
		}`
		// Error on CALLT (std.Base58CheckDecode - method of StdLib native contract) is expected, which means
		// that address.ToHash160 code was honestly generated by the compiler without any optimisations.
		prog := evalWithError(t, src, "(CALLT): runtime error: invalid memory address or nil pointer dereference")
		// Address BE bytes expected not to be present at program, which indicates that address conversion
		// was not performed at compile-time.
		require.False(t, strings.Contains(string(prog), string(h.BytesBE())))
		// On the contrary, there should be an address string.
		require.True(t, strings.Contains(string(prog), a))
	})
}

func TestInvokeAddressToFromHash160(t *testing.T) {
	a := "NQRLhCpAru9BjGsMwk67vdMwmzKMRgsnnN"
	h, err := address.StringToUint160(a)
	require.NoError(t, err)

	bc, acc := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, acc, acc)
	src := `package foo
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop"
			"github.com/nspcc-dev/neo-go/pkg/interop/lib/address"
		)
		const addr = "` + a + `"
		func ToHash160(a string) interop.Hash160 {
			return address.ToHash160(a)
		}
		func ToHash160AtCompileTime() interop.Hash160 {
			return address.ToHash160(addr)
		}
		func FromHash160(hash interop.Hash160) string {
			return address.FromHash160(hash)
		}`
	ctr := neotest.CompileSource(t, e.CommitteeHash, strings.NewReader(src), &compiler.Options{Name: "Helper"})
	e.DeployContract(t, ctr, nil)
	c := e.CommitteeInvoker(ctr.Hash)

	t.Run("ToHash160", func(t *testing.T) {
		t.Run("invalid address length", func(t *testing.T) {
			c.InvokeFail(t, "invalid address length", "toHash160", base58.CheckEncode(make([]byte, util.Uint160Size+1+1)))
		})
		t.Run("invalid prefix", func(t *testing.T) {
			c.InvokeFail(t, "invalid address prefix", "toHash160", base58.CheckEncode(append([]byte{address.NEO2Prefix}, h.BytesBE()...)))
		})
		t.Run("good", func(t *testing.T) {
			c.Invoke(t, stackitem.NewBuffer(h.BytesBE()), "toHash160", a)
		})
	})
	t.Run("ToHash160Constant", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			c.Invoke(t, stackitem.NewBuffer(h.BytesBE()), "toHash160AtCompileTime")
		})
	})
	t.Run("FromHash160", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			c.Invoke(t, stackitem.NewByteArray([]byte(a)), "fromHash160", h.BytesBE())
		})
		t.Run("invalid length", func(t *testing.T) {
			c.InvokeFail(t, "invalid Hash160 length", "fromHash160", h.BytesBE()[:15])
		})
	})
}

func TestAbort(t *testing.T) {
	src := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop/util"
	func Main() int {
		util.Abort()
		return 1
	}`
	v := vmAndCompile(t, src)
	require.Error(t, v.Run())
	require.True(t, v.HasFailed())
}

func spawnVM(t *testing.T, ic *interop.Context, src string) *vm.VM {
	b, di, err := compiler.CompileWithOptions("foo.go", strings.NewReader(src), nil)
	require.NoError(t, err)
	v := core.SpawnVM(ic)
	invokeMethod(t, testMainIdent, b.Script, v, di)
	v.LoadScriptWithFlags(b.Script, callflag.All)
	return v
}

func TestAppCall(t *testing.T) {
	srcDeep := `package foo
	func Get42() int {
		return 42
	}`
	barCtr, di, err := compiler.CompileWithOptions("bar.go", strings.NewReader(srcDeep), nil)
	require.NoError(t, err)
	mBar, err := di.ConvertToManifest(&compiler.Options{Name: "Bar"})
	require.NoError(t, err)

	barH := hash.Hash160(barCtr.Script)

	srcInner := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
	import "github.com/nspcc-dev/neo-go/pkg/interop"
	var a int = 3
	func Main(a []byte, b []byte) []byte {
		panic("Main was called")
	}
	func Append(a []byte, b []byte) []byte {
		return append(a, b...)
	}
	func Add3(n int) int {
		return a + n
	}
	func CallInner() int {
		return contract.Call(%s, "get42", contract.All).(int)
	}`
	srcInner = fmt.Sprintf(srcInner,
		fmt.Sprintf("%#v", cinterop.Hash160(barH.BytesBE())))

	inner, di, err := compiler.CompileWithOptions("foo.go", strings.NewReader(srcInner), nil)
	require.NoError(t, err)
	m, err := di.ConvertToManifest(&compiler.Options{
		Name: "Foo",
		Permissions: []manifest.Permission{
			*manifest.NewPermission(manifest.PermissionWildcard),
		},
	})
	require.NoError(t, err)

	ih := hash.Hash160(inner.Script)
	var contractGetter = func(_ *dao.Simple, h util.Uint160) (*state.Contract, error) {
		if h.Equals(ih) {
			return &state.Contract{
				ContractBase: state.ContractBase{
					Hash:     ih,
					NEF:      *inner,
					Manifest: *m,
				},
			}, nil
		} else if h.Equals(barH) {
			return &state.Contract{
				ContractBase: state.ContractBase{
					Hash:     barH,
					NEF:      *barCtr,
					Manifest: *mBar,
				},
			}, nil
		}
		return nil, errors.New("not found")
	}

	fc := fakechain.NewFakeChain()
	ic := interop.NewContext(trigger.Application, fc, dao.NewSimple(storage.NewMemoryStore(), false, false),
		interop.DefaultBaseExecFee, native.DefaultStoragePrice, contractGetter, nil, nil, nil, nil, zaptest.NewLogger(t))

	t.Run("valid script", func(t *testing.T) {
		src := getAppCallScript(fmt.Sprintf("%#v", ih.BytesBE()))
		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())

		assertResult(t, v, []byte{1, 2, 3, 4})
	})

	t.Run("callEx, valid", func(t *testing.T) {
		src := getCallExScript(fmt.Sprintf("%#v", ih.BytesBE()), "contract.ReadStates|contract.AllowCall")
		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())

		assertResult(t, v, big.NewInt(42))
	})
	t.Run("callEx, missing flags", func(t *testing.T) {
		src := getCallExScript(fmt.Sprintf("%#v", ih.BytesBE()), "contract.NoneFlag")
		v := spawnVM(t, ic, src)
		require.Error(t, v.Run())
	})

	t.Run("missing script", func(t *testing.T) {
		h := ih
		h[0] = ^h[0]

		src := getAppCallScript(fmt.Sprintf("%#v", h.BytesBE()))
		v := spawnVM(t, ic, src)
		require.Error(t, v.Run())
	})

	t.Run("convert from string constant", func(t *testing.T) {
		src := `
		package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
		const scriptHash = ` + fmt.Sprintf("%#v", string(ih.BytesBE())) + `
		func Main() []byte {
			x := []byte{1, 2}
			y := []byte{3, 4}
			result := contract.Call([]byte(scriptHash), "append", contract.All, x, y)
			return result.([]byte)
		}
		`

		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())

		assertResult(t, v, []byte{1, 2, 3, 4})
	})

	t.Run("convert from var", func(t *testing.T) {
		src := `
		package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
		func Main() []byte {
			x := []byte{1, 2}
			y := []byte{3, 4}
			var addr = []byte(` + fmt.Sprintf("%#v", string(ih.BytesBE())) + `)
			result := contract.Call(addr, "append", contract.All, x, y)
			return result.([]byte)
		}
		`

		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())

		assertResult(t, v, []byte{1, 2, 3, 4})
	})

	t.Run("InitializedGlobals", func(t *testing.T) {
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
		func Main() int {
			var addr = []byte(` + fmt.Sprintf("%#v", string(ih.BytesBE())) + `)
			result := contract.Call(addr, "add3", contract.All, 39)
			return result.(int)
		}`

		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())

		assertResult(t, v, big.NewInt(42))
	})

	t.Run("AliasPackage", func(t *testing.T) {
		src := `package foo
		import ee "github.com/nspcc-dev/neo-go/pkg/interop/contract"
		func Main() int {
			var addr = []byte(` + fmt.Sprintf("%#v", string(ih.BytesBE())) + `)
			result := ee.Call(addr, "add3", ee.All, 39)
			return result.(int)
		}`
		v := spawnVM(t, ic, src)
		require.NoError(t, v.Run())
		assertResult(t, v, big.NewInt(42))
	})
}

func getAppCallScript(h string) string {
	return `
	package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
	func Main() []byte {
		x := []byte{1, 2}
		y := []byte{3, 4}
		result := contract.Call(` + h + `, "append", contract.All, x, y)
		return result.([]byte)
	}
	`
}

func getCallExScript(h string, flags string) string {
	return `package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop/contract"
	func Main() int {
		result := contract.Call(` + h + `, "callInner", ` + flags + `)
		return result.(int)
	}`
}

func TestBuiltinDoesNotCompile(t *testing.T) {
	src := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/interop/util"
	func Main() bool {
		a := 1
		b := 2
		return util.Equals(a, b)
	}`

	v := vmAndCompile(t, src)
	ctx := v.Context()
	retCount := 0
	for op, _, err := ctx.Next(); err == nil; op, _, err = ctx.Next() {
		if ctx.IP() >= len(ctx.Program()) {
			break
		}
		if op == opcode.RET {
			retCount++
		}
	}
	require.Equal(t, 1, retCount)
}

func TestInteropPackage(t *testing.T) {
	src := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/compiler/testdata/block"
	func Main() int {
		b := block.Block{}
		a := block.GetTransactionCount(b)
		return a
	}`
	eval(t, src, big.NewInt(42))
}

func TestBuiltinPackage(t *testing.T) {
	src := `package foo
	import "github.com/nspcc-dev/neo-go/pkg/compiler/testdata/util"
	func Main() int {
		if util.Equals(1, 2) { // always returns true
			return 1
		}
		return 2
	}`
	eval(t, src, big.NewInt(1))
}

func TestLenForNil(t *testing.T) {
	src := `
	package foo
	func Main() bool {
		var a []int = nil
		return len(a) == 0
	}`

	eval(t, src, true)
}

func TestCallTConversionErrors(t *testing.T) {
	t.Run("variable hash", func(t *testing.T) {
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/neogointernal"
		func Main() int {
			var hash string
			return neogointernal.CallWithToken(hash, "method", 0).(int)
		}`
		_, err := compiler.Compile("foo.go", strings.NewReader(src))
		require.Error(t, err)
	})
	t.Run("bad hash", func(t *testing.T) {
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/neogointernal"
		func Main() int {
			return neogointernal.CallWithToken("badstring", "method", 0).(int)
		}`
		_, err := compiler.Compile("foo.go", strings.NewReader(src))
		require.Error(t, err)
	})
	t.Run("variable method", func(t *testing.T) {
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/neogointernal"
		func Main() int {
			var method string
			return neogointernal.CallWithToken("\xf5\x63\xea\x40\xbc\x28\x3d\x4d\x0e\x05\xc4\x8e\xa3\x05\xb3\xf2\xa0\x73\x40\xef", method, 0).(int)
		}`
		_, err := compiler.Compile("foo.go", strings.NewReader(src))
		require.Error(t, err)
	})
	t.Run("variable flags", func(t *testing.T) {
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/neogointernal"
		func Main() {
			var flags int
			neogointernal.CallWithTokenNoRet("\xf5\x63\xea\x40\xbc\x28\x3d\x4d\x0e\x05\xc4\x8e\xa3\x05\xb3\xf2\xa0\x73\x40\xef", "method", flags)
		}`
		_, err := compiler.Compile("foo.go", strings.NewReader(src))
		require.Error(t, err)
	})
}

func TestCallWithVersion(t *testing.T) {
	bc, acc := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, acc, acc)
	src := `package foo
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop"
			"github.com/nspcc-dev/neo-go/pkg/interop/contract"
			util "github.com/nspcc-dev/neo-go/pkg/interop/lib/contract"
		)
		func CallWithVersion(hash interop.Hash160, version int, method string) interface{} {
			return util.CallWithVersion(hash, version, method, contract.All)
		}`
	ctr := neotest.CompileSource(t, e.CommitteeHash, strings.NewReader(src), &compiler.Options{Name: "Helper"})
	e.DeployContract(t, ctr, nil)
	c := e.CommitteeInvoker(ctr.Hash)

	policyH := state.CreateNativeContractHash(nativenames.Policy)
	t.Run("good", func(t *testing.T) {
		c.Invoke(t, e.Chain.GetBaseExecFee(), "callWithVersion", policyH.BytesBE(), 0, "getExecFeeFactor")
	})
	t.Run("unknown contract", func(t *testing.T) {
		c.InvokeFail(t, "unknown contract", "callWithVersion", util.Uint160{1, 2, 3}.BytesBE(), 0, "getExecFeeFactor")
	})
	t.Run("invalid version", func(t *testing.T) {
		c.InvokeFail(t, "contract version mismatch", "callWithVersion", policyH.BytesBE(), 1, "getExecFeeFactor")
	})
}

func TestForcedNotifyArgumentsConversion(t *testing.T) {
	const methodWithEllipsis = "withEllipsis"
	const methodWithoutEllipsis = "withoutEllipsis"
	check := func(t *testing.T, method string, targetSCParamTypes []smartcontract.ParamType, expectedVMParamTypes []stackitem.Type, noEventsCheck bool) {
		bc, acc := chain.NewSingle(t)
		e := neotest.NewExecutor(t, bc, acc, acc)
		src := `package foo
		import "github.com/nspcc-dev/neo-go/pkg/interop/runtime"
		const arg4 = 4			// Const value.
		func WithoutEllipsis() {
			var arg0 int		// Default value.
			var arg1 int = 1	// Initialized value.
			arg2 := 2			// Short decl.
			var arg3 int
			arg3 = 3			// Declare first, change value afterwards.
			runtime.Notify("withoutEllipsis", arg0, arg1, arg2, arg3, arg4, 5, f(6))	// The fifth argument is basic literal.
		}
		func WithEllipsis() {
			arg := []interface{}{0, 1, f(2), 3, 4, 5, 6}
			runtime.Notify("withEllipsis", arg...)
		}
		func f(i int) int {
			return i
		}`
		count := len(targetSCParamTypes)
		if count != len(expectedVMParamTypes) {
			t.Fatalf("parameters count mismatch: %d vs %d", count, len(expectedVMParamTypes))
		}
		scParams := make([]manifest.Parameter, len(targetSCParamTypes))
		vmParams := make([]stackitem.Item, len(expectedVMParamTypes))
		for i := range scParams {
			scParams[i] = manifest.Parameter{
				Name: strconv.Itoa(i),
				Type: targetSCParamTypes[i],
			}
			defaultValue := stackitem.NewBigInteger(big.NewInt(int64(i)))
			var (
				val stackitem.Item
				err error
			)
			if expectedVMParamTypes[i] == stackitem.IntegerT {
				val = defaultValue
			} else {
				val, err = defaultValue.Convert(expectedVMParamTypes[i]) // exactly the same conversion should be emitted by compiler and performed by the contract code.
				require.NoError(t, err)
			}
			vmParams[i] = val
		}
		ctr := neotest.CompileSource(t, e.CommitteeHash, strings.NewReader(src), &compiler.Options{
			Name: "Helper",
			ContractEvents: []manifest.Event{
				{
					Name:       methodWithoutEllipsis,
					Parameters: scParams,
				},
				{
					Name:       methodWithEllipsis,
					Parameters: scParams,
				},
			},
			NoEventsCheck: noEventsCheck,
		})
		e.DeployContract(t, ctr, nil)
		c := e.CommitteeInvoker(ctr.Hash)

		t.Run(method, func(t *testing.T) {
			h := c.Invoke(t, stackitem.Null{}, method)
			aer := c.GetTxExecResult(t, h)
			require.Equal(t, 1, len(aer.Events))
			require.Equal(t, stackitem.NewArray(vmParams), aer.Events[0].Item)
		})
	}
	checkSingleType := func(t *testing.T, method string, targetSCEventType smartcontract.ParamType, expectedVMType stackitem.Type, noEventsCheck ...bool) {
		count := 7
		scParams := make([]smartcontract.ParamType, count)
		vmParams := make([]stackitem.Type, count)
		for i := range scParams {
			scParams[i] = targetSCEventType
			vmParams[i] = expectedVMType
		}
		var noEvents bool
		if len(noEventsCheck) > 0 {
			noEvents = noEventsCheck[0]
		}
		check(t, method, scParams, vmParams, noEvents)
	}

	t.Run("good, single type, default values", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.IntegerType, stackitem.IntegerT)
	})
	t.Run("good, single type, conversion to BooleanT", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.BoolType, stackitem.BooleanT)
	})
	t.Run("good, single type, Hash160Type->ByteArray", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.Hash160Type, stackitem.ByteArrayT)
	})
	t.Run("good, single type, Hash256Type->ByteArray", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.Hash256Type, stackitem.ByteArrayT)
	})
	t.Run("good, single type, Signature->ByteArray", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.SignatureType, stackitem.ByteArrayT)
	})
	t.Run("good, single type, String->ByteArray", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.StringType, stackitem.ByteArrayT) // Special case, runtime.Notify will convert any Buffer to ByteArray.
	})
	t.Run("good, single type, PublicKeyType->ByteArray", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.PublicKeyType, stackitem.ByteArrayT)
	})
	t.Run("good, single type, AnyType->do not change initial type", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.AnyType, stackitem.IntegerT) // Special case, compiler should leave the type "as is" and do not emit conversion code.
	})
	// Test for InteropInterface->... is missing, because we don't enforce conversion to stackitem.InteropInterface,
	// but compiler still checks these notifications against expected manifest.
	t.Run("good, multiple types, check the conversion order", func(t *testing.T) {
		check(t, methodWithoutEllipsis, []smartcontract.ParamType{
			smartcontract.IntegerType,
			smartcontract.BoolType,
			smartcontract.ByteArrayType,
			smartcontract.PublicKeyType,
			smartcontract.Hash160Type,
			smartcontract.AnyType, // leave initial type
			smartcontract.StringType,
		}, []stackitem.Type{
			stackitem.IntegerT,
			stackitem.BooleanT,
			stackitem.ByteArrayT,
			stackitem.ByteArrayT,
			stackitem.ByteArrayT,
			stackitem.IntegerT, // leave initial type
			stackitem.ByteArrayT,
		}, false)
	})
	t.Run("with ellipsis, do not emit conversion code", func(t *testing.T) {
		checkSingleType(t, methodWithEllipsis, smartcontract.IntegerType, stackitem.IntegerT)
		checkSingleType(t, methodWithEllipsis, smartcontract.BoolType, stackitem.IntegerT)
		checkSingleType(t, methodWithEllipsis, smartcontract.ByteArrayType, stackitem.IntegerT)
	})
	t.Run("no events check => no conversion code", func(t *testing.T) {
		checkSingleType(t, methodWithoutEllipsis, smartcontract.PublicKeyType, stackitem.IntegerT, true)
	})
}
