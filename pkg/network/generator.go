package network

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"unsafe"
)

type Generator struct {
	data        []byte
	dataSize    uint32
	position    uint32
	mapStruct   map[string]interface{}
	arrayStruct []interface{}
}

func NewGenerator(fuzzData []byte) *Generator {
	return &Generator{
		data:     fuzzData,
		dataSize: uint32(len(fuzzData)),
	}
}

func (g *Generator) fillAny(any reflect.Value) error {
	switch any.Kind() {
	case reflect.Struct:
		for i := 0; i < any.NumField(); i++ {
			err := g.fillAny(any.Field(i))
			if err != nil {
				return err
			}
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		newInt, err := g.GenerateInt()
		if err != nil {
			return err
		}
		any.SetInt(int64(newInt))

	case reflect.Ptr:
		any.Set(reflect.New(any.Type().Elem()))
		err := g.fillAny(any.Elem())
		if err != nil {
			return err
		}

	case reflect.String:
		str, err := g.GenerateString()
		if err != nil {
			return err
		}
		any.SetString(str)

	case reflect.Float32:
		newFloat, err := g.GenerateFloat32()
		if err != nil {
			return err
		}
		any.SetFloat(float64(newFloat))

	case reflect.Float64:
		newFloat, err := g.GenerateFloat64()
		if err != nil {
			return err
		}
		any.SetFloat(float64(newFloat))

	case reflect.Complex64:
		newFloatReal, err := g.GenerateFloat32()
		if err != nil {
			return err
		}
		newFloatImag, err := g.GenerateFloat32()
		if err != nil {
			return err
		}
		any.SetComplex(complex(float64(newFloatReal), float64(newFloatImag)))

	case reflect.Complex128:
		newFloatReal, err := g.GenerateFloat64()
		if err != nil {
			return err
		}
		newFloatImag, err := g.GenerateFloat64()
		if err != nil {
			return err
		}
		any.SetComplex(complex(newFloatReal, newFloatImag))

	case reflect.Map:
		any.Set(reflect.MakeMap(any.Type()))

		mapLen, err := g.GenerateUInt32()
		if err != nil {
			return err
		}

		for i := uint32(0); i < mapLen; i++ {
			key := fmt.Sprintf("key%d", i)
			mapValue := reflect.New(any.Type().Elem())
			err := g.fillAny(mapValue.Elem())
			if err != nil {
				return err
			}
			any.SetMapIndex(reflect.ValueOf(key), mapValue.Elem())
		}

	case reflect.Array:
		any.Set(reflect.New(any.Type()).Elem())
		arrayLen := any.Len()
		for i := 0; i < arrayLen; i++ {
			arrayValue := reflect.New(any.Type().Elem())
			err := g.fillAny(arrayValue.Elem())
			if err != nil {
				return err
			}
			any.Index(i).Set(arrayValue.Elem())
		}

	case reflect.Bool:
		newBool, err := g.GenerateBool()
		if err != nil {
			return err
		}
		any.SetBool(newBool)

	case reflect.Uint8:
		newUInt8, err := g.GenerateUInt8()
		if err != nil {
			return err
		}
		any.SetUint(uint64(newUInt8))

	case reflect.Uint16:
		newUInt16, err := g.GenerateUInt16()
		if err != nil {
			return err
		}
		any.SetUint(uint64(newUInt16))

	case reflect.Uint, reflect.Uint32:
		newUInt32, err := g.GenerateUInt32()
		if err != nil {
			return err
		}
		any.SetUint(uint64(newUInt32))

	case reflect.Uint64, reflect.Uintptr:
		newUInt64, err := g.GenerateUInt64()
		if err != nil {
			return err
		}
		any.SetUint(newUInt64)

	case reflect.Chan:
		maxSize := uint8(50)
		size, err := g.GenerateUInt8()
		if err != nil {
			return err
		}
		size = size % maxSize
		channel := reflect.MakeChan(any.Type(), int(size))
		for i := 0; i < int(size); i++ {
			elem := reflect.New(channel.Type().Elem()).Elem()
			err = g.fillAny(elem)
			if err != nil {
				return err
			}
			channel.Send(elem)
		}
		any.Set(channel)

	case reflect.Slice:
		maxSize := uint8(50)
		size, err := g.GenerateUInt8()
		if err != nil {
			return err
		}
		size = size % maxSize

		slice := reflect.MakeSlice(any.Type(), int(size), int(size))
		for i := 0; i < int(size); i++ {
			err := g.fillAny(slice.Index(i))
			if err != nil {
				return err
			}
		}
		if any.CanSet() {
			any.Set(slice)
		}

	case reflect.UnsafePointer:
		addr, err := g.GenerateUInt64()
		if err != nil {
			return err
		}
		any.SetPointer(unsafe.Pointer(uintptr(addr)))

	case reflect.Interface:

	default:
		panic("unhandled default case")
	}
	return nil
}

func (g *Generator) GenerateStruct(targetStruct interface{}) error {
	e := reflect.ValueOf(targetStruct).Elem()
	return g.fillAny(e)
}

func (g *Generator) GenerateInt() (int, error) {
	if g.position >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}
	result := int(g.data[g.position])
	g.position++
	return result, nil
}

func (g *Generator) GenerateUInt32() (uint32, error) {
	if g.position+3 >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}
	result := uint32(0)
	for i := 0; i < 4; i++ {
		result = result<<8 | uint32(g.data[g.position])
		g.position++
	}
	return result, nil
}

func (g *Generator) GenerateString() (string, error) {
	maxStrLength := uint32(1000)

	if g.position >= g.dataSize {
		return "nil", errors.New("the data bytes are over")
	}

	length, err := g.GenerateUInt32()
	if err != nil {
		return "nil", errors.New("the data bytes are over")
	}
	length = length % maxStrLength

	startBytePos := g.position
	if startBytePos >= g.dataSize {
		return "nil", errors.New("the data bytes are over")
	}

	if startBytePos+length > g.dataSize {
		return "nil", errors.New("the data bytes are over")
	}

	if startBytePos > startBytePos+length {
		return "nil", errors.New("overflow")
	}

	g.position = startBytePos + length
	result := string(g.data[startBytePos:g.position])

	return result, nil
}

func (g *Generator) GenerateFloat32() (float32, error) {
	if g.position+3 >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}

	bits := uint32(0)
	for i := 0; i < 4; i++ {
		bits = bits<<8 | uint32(g.data[g.position])
		g.position++
	}

	floatBits := uint32(math.Float32bits(math.Float32frombits(bits)))
	result := math.Float32frombits(floatBits)

	return result, nil
}

func (g *Generator) GenerateFloat64() (float64, error) {
	if g.position+7 >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}

	bits := uint64(0)
	for i := 0; i < 8; i++ {
		bits = bits<<8 | uint64(g.data[g.position])
		g.position++
	}

	result := math.Float64frombits(bits)

	return result, nil
}

func (g *Generator) GenerateBool() (bool, error) {
	if g.position >= g.dataSize {
		return false, errors.New("the data bytes are over")
	}
	result := g.data[g.position]%2 == 0
	g.position++
	return result, nil
}

func (g *Generator) GenerateUInt8() (uint8, error) {
	if g.position >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}
	result := g.data[g.position]
	g.position++
	return result, nil
}

func (g *Generator) GenerateUInt16() (uint16, error) {
	if g.position+1 >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}
	result := uint16(0)
	for i := 0; i < 2; i++ {
		result = result<<8 | uint16(g.data[g.position])
		g.position++
	}
	return result, nil
}

func (g *Generator) GenerateUInt64() (uint64, error) {
	if g.position+7 >= g.dataSize {
		return 0, errors.New("the data bytes are over")
	}
	result := uint64(0)
	for i := 0; i < 8; i++ {
		result = result<<8 | uint64(g.data[g.position])
		g.position++
	}
	return result, nil
}
