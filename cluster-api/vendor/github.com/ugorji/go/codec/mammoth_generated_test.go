// Copyright (c) 2012-2015 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

// ************************************************************
// DO NOT EDIT.
// THIS FILE IS AUTO-GENERATED from mammoth-test.go.tmpl
// ************************************************************

package codec

import "testing"

// TestMammoth has all the different paths optimized in fast-path
// It has all the primitives, slices and maps.
//
// For each of those types, it has a pointer and a non-pointer field.

type TestMammoth struct {
	FIntf       interface{}
	FptrIntf    *interface{}
	FString     string
	FptrString  *string
	FFloat32    float32
	FptrFloat32 *float32
	FFloat64    float64
	FptrFloat64 *float64
	FUint       uint
	FptrUint    *uint
	FUint8      uint8
	FptrUint8   *uint8
	FUint16     uint16
	FptrUint16  *uint16
	FUint32     uint32
	FptrUint32  *uint32
	FUint64     uint64
	FptrUint64  *uint64
	FUintptr    uintptr
	FptrUintptr *uintptr
	FInt        int
	FptrInt     *int
	FInt8       int8
	FptrInt8    *int8
	FInt16      int16
	FptrInt16   *int16
	FInt32      int32
	FptrInt32   *int32
	FInt64      int64
	FptrInt64   *int64
	FBool       bool
	FptrBool    *bool

	FSliceIntf       []interface{}
	FptrSliceIntf    *[]interface{}
	FSliceString     []string
	FptrSliceString  *[]string
	FSliceFloat32    []float32
	FptrSliceFloat32 *[]float32
	FSliceFloat64    []float64
	FptrSliceFloat64 *[]float64
	FSliceUint       []uint
	FptrSliceUint    *[]uint
	FSliceUint16     []uint16
	FptrSliceUint16  *[]uint16
	FSliceUint32     []uint32
	FptrSliceUint32  *[]uint32
	FSliceUint64     []uint64
	FptrSliceUint64  *[]uint64
	FSliceUintptr    []uintptr
	FptrSliceUintptr *[]uintptr
	FSliceInt        []int
	FptrSliceInt     *[]int
	FSliceInt8       []int8
	FptrSliceInt8    *[]int8
	FSliceInt16      []int16
	FptrSliceInt16   *[]int16
	FSliceInt32      []int32
	FptrSliceInt32   *[]int32
	FSliceInt64      []int64
	FptrSliceInt64   *[]int64
	FSliceBool       []bool
	FptrSliceBool    *[]bool

	FMapIntfIntf          map[interface{}]interface{}
	FptrMapIntfIntf       *map[interface{}]interface{}
	FMapIntfString        map[interface{}]string
	FptrMapIntfString     *map[interface{}]string
	FMapIntfUint          map[interface{}]uint
	FptrMapIntfUint       *map[interface{}]uint
	FMapIntfUint8         map[interface{}]uint8
	FptrMapIntfUint8      *map[interface{}]uint8
	FMapIntfUint16        map[interface{}]uint16
	FptrMapIntfUint16     *map[interface{}]uint16
	FMapIntfUint32        map[interface{}]uint32
	FptrMapIntfUint32     *map[interface{}]uint32
	FMapIntfUint64        map[interface{}]uint64
	FptrMapIntfUint64     *map[interface{}]uint64
	FMapIntfUintptr       map[interface{}]uintptr
	FptrMapIntfUintptr    *map[interface{}]uintptr
	FMapIntfInt           map[interface{}]int
	FptrMapIntfInt        *map[interface{}]int
	FMapIntfInt8          map[interface{}]int8
	FptrMapIntfInt8       *map[interface{}]int8
	FMapIntfInt16         map[interface{}]int16
	FptrMapIntfInt16      *map[interface{}]int16
	FMapIntfInt32         map[interface{}]int32
	FptrMapIntfInt32      *map[interface{}]int32
	FMapIntfInt64         map[interface{}]int64
	FptrMapIntfInt64      *map[interface{}]int64
	FMapIntfFloat32       map[interface{}]float32
	FptrMapIntfFloat32    *map[interface{}]float32
	FMapIntfFloat64       map[interface{}]float64
	FptrMapIntfFloat64    *map[interface{}]float64
	FMapIntfBool          map[interface{}]bool
	FptrMapIntfBool       *map[interface{}]bool
	FMapStringIntf        map[string]interface{}
	FptrMapStringIntf     *map[string]interface{}
	FMapStringString      map[string]string
	FptrMapStringString   *map[string]string
	FMapStringUint        map[string]uint
	FptrMapStringUint     *map[string]uint
	FMapStringUint8       map[string]uint8
	FptrMapStringUint8    *map[string]uint8
	FMapStringUint16      map[string]uint16
	FptrMapStringUint16   *map[string]uint16
	FMapStringUint32      map[string]uint32
	FptrMapStringUint32   *map[string]uint32
	FMapStringUint64      map[string]uint64
	FptrMapStringUint64   *map[string]uint64
	FMapStringUintptr     map[string]uintptr
	FptrMapStringUintptr  *map[string]uintptr
	FMapStringInt         map[string]int
	FptrMapStringInt      *map[string]int
	FMapStringInt8        map[string]int8
	FptrMapStringInt8     *map[string]int8
	FMapStringInt16       map[string]int16
	FptrMapStringInt16    *map[string]int16
	FMapStringInt32       map[string]int32
	FptrMapStringInt32    *map[string]int32
	FMapStringInt64       map[string]int64
	FptrMapStringInt64    *map[string]int64
	FMapStringFloat32     map[string]float32
	FptrMapStringFloat32  *map[string]float32
	FMapStringFloat64     map[string]float64
	FptrMapStringFloat64  *map[string]float64
	FMapStringBool        map[string]bool
	FptrMapStringBool     *map[string]bool
	FMapFloat32Intf       map[float32]interface{}
	FptrMapFloat32Intf    *map[float32]interface{}
	FMapFloat32String     map[float32]string
	FptrMapFloat32String  *map[float32]string
	FMapFloat32Uint       map[float32]uint
	FptrMapFloat32Uint    *map[float32]uint
	FMapFloat32Uint8      map[float32]uint8
	FptrMapFloat32Uint8   *map[float32]uint8
	FMapFloat32Uint16     map[float32]uint16
	FptrMapFloat32Uint16  *map[float32]uint16
	FMapFloat32Uint32     map[float32]uint32
	FptrMapFloat32Uint32  *map[float32]uint32
	FMapFloat32Uint64     map[float32]uint64
	FptrMapFloat32Uint64  *map[float32]uint64
	FMapFloat32Uintptr    map[float32]uintptr
	FptrMapFloat32Uintptr *map[float32]uintptr
	FMapFloat32Int        map[float32]int
	FptrMapFloat32Int     *map[float32]int
	FMapFloat32Int8       map[float32]int8
	FptrMapFloat32Int8    *map[float32]int8
	FMapFloat32Int16      map[float32]int16
	FptrMapFloat32Int16   *map[float32]int16
	FMapFloat32Int32      map[float32]int32
	FptrMapFloat32Int32   *map[float32]int32
	FMapFloat32Int64      map[float32]int64
	FptrMapFloat32Int64   *map[float32]int64
	FMapFloat32Float32    map[float32]float32
	FptrMapFloat32Float32 *map[float32]float32
	FMapFloat32Float64    map[float32]float64
	FptrMapFloat32Float64 *map[float32]float64
	FMapFloat32Bool       map[float32]bool
	FptrMapFloat32Bool    *map[float32]bool
	FMapFloat64Intf       map[float64]interface{}
	FptrMapFloat64Intf    *map[float64]interface{}
	FMapFloat64String     map[float64]string
	FptrMapFloat64String  *map[float64]string
	FMapFloat64Uint       map[float64]uint
	FptrMapFloat64Uint    *map[float64]uint
	FMapFloat64Uint8      map[float64]uint8
	FptrMapFloat64Uint8   *map[float64]uint8
	FMapFloat64Uint16     map[float64]uint16
	FptrMapFloat64Uint16  *map[float64]uint16
	FMapFloat64Uint32     map[float64]uint32
	FptrMapFloat64Uint32  *map[float64]uint32
	FMapFloat64Uint64     map[float64]uint64
	FptrMapFloat64Uint64  *map[float64]uint64
	FMapFloat64Uintptr    map[float64]uintptr
	FptrMapFloat64Uintptr *map[float64]uintptr
	FMapFloat64Int        map[float64]int
	FptrMapFloat64Int     *map[float64]int
	FMapFloat64Int8       map[float64]int8
	FptrMapFloat64Int8    *map[float64]int8
	FMapFloat64Int16      map[float64]int16
	FptrMapFloat64Int16   *map[float64]int16
	FMapFloat64Int32      map[float64]int32
	FptrMapFloat64Int32   *map[float64]int32
	FMapFloat64Int64      map[float64]int64
	FptrMapFloat64Int64   *map[float64]int64
	FMapFloat64Float32    map[float64]float32
	FptrMapFloat64Float32 *map[float64]float32
	FMapFloat64Float64    map[float64]float64
	FptrMapFloat64Float64 *map[float64]float64
	FMapFloat64Bool       map[float64]bool
	FptrMapFloat64Bool    *map[float64]bool
	FMapUintIntf          map[uint]interface{}
	FptrMapUintIntf       *map[uint]interface{}
	FMapUintString        map[uint]string
	FptrMapUintString     *map[uint]string
	FMapUintUint          map[uint]uint
	FptrMapUintUint       *map[uint]uint
	FMapUintUint8         map[uint]uint8
	FptrMapUintUint8      *map[uint]uint8
	FMapUintUint16        map[uint]uint16
	FptrMapUintUint16     *map[uint]uint16
	FMapUintUint32        map[uint]uint32
	FptrMapUintUint32     *map[uint]uint32
	FMapUintUint64        map[uint]uint64
	FptrMapUintUint64     *map[uint]uint64
	FMapUintUintptr       map[uint]uintptr
	FptrMapUintUintptr    *map[uint]uintptr
	FMapUintInt           map[uint]int
	FptrMapUintInt        *map[uint]int
	FMapUintInt8          map[uint]int8
	FptrMapUintInt8       *map[uint]int8
	FMapUintInt16         map[uint]int16
	FptrMapUintInt16      *map[uint]int16
	FMapUintInt32         map[uint]int32
	FptrMapUintInt32      *map[uint]int32
	FMapUintInt64         map[uint]int64
	FptrMapUintInt64      *map[uint]int64
	FMapUintFloat32       map[uint]float32
	FptrMapUintFloat32    *map[uint]float32
	FMapUintFloat64       map[uint]float64
	FptrMapUintFloat64    *map[uint]float64
	FMapUintBool          map[uint]bool
	FptrMapUintBool       *map[uint]bool
	FMapUint8Intf         map[uint8]interface{}
	FptrMapUint8Intf      *map[uint8]interface{}
	FMapUint8String       map[uint8]string
	FptrMapUint8String    *map[uint8]string
	FMapUint8Uint         map[uint8]uint
	FptrMapUint8Uint      *map[uint8]uint
	FMapUint8Uint8        map[uint8]uint8
	FptrMapUint8Uint8     *map[uint8]uint8
	FMapUint8Uint16       map[uint8]uint16
	FptrMapUint8Uint16    *map[uint8]uint16
	FMapUint8Uint32       map[uint8]uint32
	FptrMapUint8Uint32    *map[uint8]uint32
	FMapUint8Uint64       map[uint8]uint64
	FptrMapUint8Uint64    *map[uint8]uint64
	FMapUint8Uintptr      map[uint8]uintptr
	FptrMapUint8Uintptr   *map[uint8]uintptr
	FMapUint8Int          map[uint8]int
	FptrMapUint8Int       *map[uint8]int
	FMapUint8Int8         map[uint8]int8
	FptrMapUint8Int8      *map[uint8]int8
	FMapUint8Int16        map[uint8]int16
	FptrMapUint8Int16     *map[uint8]int16
	FMapUint8Int32        map[uint8]int32
	FptrMapUint8Int32     *map[uint8]int32
	FMapUint8Int64        map[uint8]int64
	FptrMapUint8Int64     *map[uint8]int64
	FMapUint8Float32      map[uint8]float32
	FptrMapUint8Float32   *map[uint8]float32
	FMapUint8Float64      map[uint8]float64
	FptrMapUint8Float64   *map[uint8]float64
	FMapUint8Bool         map[uint8]bool
	FptrMapUint8Bool      *map[uint8]bool
	FMapUint16Intf        map[uint16]interface{}
	FptrMapUint16Intf     *map[uint16]interface{}
	FMapUint16String      map[uint16]string
	FptrMapUint16String   *map[uint16]string
	FMapUint16Uint        map[uint16]uint
	FptrMapUint16Uint     *map[uint16]uint
	FMapUint16Uint8       map[uint16]uint8
	FptrMapUint16Uint8    *map[uint16]uint8
	FMapUint16Uint16      map[uint16]uint16
	FptrMapUint16Uint16   *map[uint16]uint16
	FMapUint16Uint32      map[uint16]uint32
	FptrMapUint16Uint32   *map[uint16]uint32
	FMapUint16Uint64      map[uint16]uint64
	FptrMapUint16Uint64   *map[uint16]uint64
	FMapUint16Uintptr     map[uint16]uintptr
	FptrMapUint16Uintptr  *map[uint16]uintptr
	FMapUint16Int         map[uint16]int
	FptrMapUint16Int      *map[uint16]int
	FMapUint16Int8        map[uint16]int8
	FptrMapUint16Int8     *map[uint16]int8
	FMapUint16Int16       map[uint16]int16
	FptrMapUint16Int16    *map[uint16]int16
	FMapUint16Int32       map[uint16]int32
	FptrMapUint16Int32    *map[uint16]int32
	FMapUint16Int64       map[uint16]int64
	FptrMapUint16Int64    *map[uint16]int64
	FMapUint16Float32     map[uint16]float32
	FptrMapUint16Float32  *map[uint16]float32
	FMapUint16Float64     map[uint16]float64
	FptrMapUint16Float64  *map[uint16]float64
	FMapUint16Bool        map[uint16]bool
	FptrMapUint16Bool     *map[uint16]bool
	FMapUint32Intf        map[uint32]interface{}
	FptrMapUint32Intf     *map[uint32]interface{}
	FMapUint32String      map[uint32]string
	FptrMapUint32String   *map[uint32]string
	FMapUint32Uint        map[uint32]uint
	FptrMapUint32Uint     *map[uint32]uint
	FMapUint32Uint8       map[uint32]uint8
	FptrMapUint32Uint8    *map[uint32]uint8
	FMapUint32Uint16      map[uint32]uint16
	FptrMapUint32Uint16   *map[uint32]uint16
	FMapUint32Uint32      map[uint32]uint32
	FptrMapUint32Uint32   *map[uint32]uint32
	FMapUint32Uint64      map[uint32]uint64
	FptrMapUint32Uint64   *map[uint32]uint64
	FMapUint32Uintptr     map[uint32]uintptr
	FptrMapUint32Uintptr  *map[uint32]uintptr
	FMapUint32Int         map[uint32]int
	FptrMapUint32Int      *map[uint32]int
	FMapUint32Int8        map[uint32]int8
	FptrMapUint32Int8     *map[uint32]int8
	FMapUint32Int16       map[uint32]int16
	FptrMapUint32Int16    *map[uint32]int16
	FMapUint32Int32       map[uint32]int32
	FptrMapUint32Int32    *map[uint32]int32
	FMapUint32Int64       map[uint32]int64
	FptrMapUint32Int64    *map[uint32]int64
	FMapUint32Float32     map[uint32]float32
	FptrMapUint32Float32  *map[uint32]float32
	FMapUint32Float64     map[uint32]float64
	FptrMapUint32Float64  *map[uint32]float64
	FMapUint32Bool        map[uint32]bool
	FptrMapUint32Bool     *map[uint32]bool
	FMapUint64Intf        map[uint64]interface{}
	FptrMapUint64Intf     *map[uint64]interface{}
	FMapUint64String      map[uint64]string
	FptrMapUint64String   *map[uint64]string
	FMapUint64Uint        map[uint64]uint
	FptrMapUint64Uint     *map[uint64]uint
	FMapUint64Uint8       map[uint64]uint8
	FptrMapUint64Uint8    *map[uint64]uint8
	FMapUint64Uint16      map[uint64]uint16
	FptrMapUint64Uint16   *map[uint64]uint16
	FMapUint64Uint32      map[uint64]uint32
	FptrMapUint64Uint32   *map[uint64]uint32
	FMapUint64Uint64      map[uint64]uint64
	FptrMapUint64Uint64   *map[uint64]uint64
	FMapUint64Uintptr     map[uint64]uintptr
	FptrMapUint64Uintptr  *map[uint64]uintptr
	FMapUint64Int         map[uint64]int
	FptrMapUint64Int      *map[uint64]int
	FMapUint64Int8        map[uint64]int8
	FptrMapUint64Int8     *map[uint64]int8
	FMapUint64Int16       map[uint64]int16
	FptrMapUint64Int16    *map[uint64]int16
	FMapUint64Int32       map[uint64]int32
	FptrMapUint64Int32    *map[uint64]int32
	FMapUint64Int64       map[uint64]int64
	FptrMapUint64Int64    *map[uint64]int64
	FMapUint64Float32     map[uint64]float32
	FptrMapUint64Float32  *map[uint64]float32
	FMapUint64Float64     map[uint64]float64
	FptrMapUint64Float64  *map[uint64]float64
	FMapUint64Bool        map[uint64]bool
	FptrMapUint64Bool     *map[uint64]bool
	FMapUintptrIntf       map[uintptr]interface{}
	FptrMapUintptrIntf    *map[uintptr]interface{}
	FMapUintptrString     map[uintptr]string
	FptrMapUintptrString  *map[uintptr]string
	FMapUintptrUint       map[uintptr]uint
	FptrMapUintptrUint    *map[uintptr]uint
	FMapUintptrUint8      map[uintptr]uint8
	FptrMapUintptrUint8   *map[uintptr]uint8
	FMapUintptrUint16     map[uintptr]uint16
	FptrMapUintptrUint16  *map[uintptr]uint16
	FMapUintptrUint32     map[uintptr]uint32
	FptrMapUintptrUint32  *map[uintptr]uint32
	FMapUintptrUint64     map[uintptr]uint64
	FptrMapUintptrUint64  *map[uintptr]uint64
	FMapUintptrUintptr    map[uintptr]uintptr
	FptrMapUintptrUintptr *map[uintptr]uintptr
	FMapUintptrInt        map[uintptr]int
	FptrMapUintptrInt     *map[uintptr]int
	FMapUintptrInt8       map[uintptr]int8
	FptrMapUintptrInt8    *map[uintptr]int8
	FMapUintptrInt16      map[uintptr]int16
	FptrMapUintptrInt16   *map[uintptr]int16
	FMapUintptrInt32      map[uintptr]int32
	FptrMapUintptrInt32   *map[uintptr]int32
	FMapUintptrInt64      map[uintptr]int64
	FptrMapUintptrInt64   *map[uintptr]int64
	FMapUintptrFloat32    map[uintptr]float32
	FptrMapUintptrFloat32 *map[uintptr]float32
	FMapUintptrFloat64    map[uintptr]float64
	FptrMapUintptrFloat64 *map[uintptr]float64
	FMapUintptrBool       map[uintptr]bool
	FptrMapUintptrBool    *map[uintptr]bool
	FMapIntIntf           map[int]interface{}
	FptrMapIntIntf        *map[int]interface{}
	FMapIntString         map[int]string
	FptrMapIntString      *map[int]string
	FMapIntUint           map[int]uint
	FptrMapIntUint        *map[int]uint
	FMapIntUint8          map[int]uint8
	FptrMapIntUint8       *map[int]uint8
	FMapIntUint16         map[int]uint16
	FptrMapIntUint16      *map[int]uint16
	FMapIntUint32         map[int]uint32
	FptrMapIntUint32      *map[int]uint32
	FMapIntUint64         map[int]uint64
	FptrMapIntUint64      *map[int]uint64
	FMapIntUintptr        map[int]uintptr
	FptrMapIntUintptr     *map[int]uintptr
	FMapIntInt            map[int]int
	FptrMapIntInt         *map[int]int
	FMapIntInt8           map[int]int8
	FptrMapIntInt8        *map[int]int8
	FMapIntInt16          map[int]int16
	FptrMapIntInt16       *map[int]int16
	FMapIntInt32          map[int]int32
	FptrMapIntInt32       *map[int]int32
	FMapIntInt64          map[int]int64
	FptrMapIntInt64       *map[int]int64
	FMapIntFloat32        map[int]float32
	FptrMapIntFloat32     *map[int]float32
	FMapIntFloat64        map[int]float64
	FptrMapIntFloat64     *map[int]float64
	FMapIntBool           map[int]bool
	FptrMapIntBool        *map[int]bool
	FMapInt8Intf          map[int8]interface{}
	FptrMapInt8Intf       *map[int8]interface{}
	FMapInt8String        map[int8]string
	FptrMapInt8String     *map[int8]string
	FMapInt8Uint          map[int8]uint
	FptrMapInt8Uint       *map[int8]uint
	FMapInt8Uint8         map[int8]uint8
	FptrMapInt8Uint8      *map[int8]uint8
	FMapInt8Uint16        map[int8]uint16
	FptrMapInt8Uint16     *map[int8]uint16
	FMapInt8Uint32        map[int8]uint32
	FptrMapInt8Uint32     *map[int8]uint32
	FMapInt8Uint64        map[int8]uint64
	FptrMapInt8Uint64     *map[int8]uint64
	FMapInt8Uintptr       map[int8]uintptr
	FptrMapInt8Uintptr    *map[int8]uintptr
	FMapInt8Int           map[int8]int
	FptrMapInt8Int        *map[int8]int
	FMapInt8Int8          map[int8]int8
	FptrMapInt8Int8       *map[int8]int8
	FMapInt8Int16         map[int8]int16
	FptrMapInt8Int16      *map[int8]int16
	FMapInt8Int32         map[int8]int32
	FptrMapInt8Int32      *map[int8]int32
	FMapInt8Int64         map[int8]int64
	FptrMapInt8Int64      *map[int8]int64
	FMapInt8Float32       map[int8]float32
	FptrMapInt8Float32    *map[int8]float32
	FMapInt8Float64       map[int8]float64
	FptrMapInt8Float64    *map[int8]float64
	FMapInt8Bool          map[int8]bool
	FptrMapInt8Bool       *map[int8]bool
	FMapInt16Intf         map[int16]interface{}
	FptrMapInt16Intf      *map[int16]interface{}
	FMapInt16String       map[int16]string
	FptrMapInt16String    *map[int16]string
	FMapInt16Uint         map[int16]uint
	FptrMapInt16Uint      *map[int16]uint
	FMapInt16Uint8        map[int16]uint8
	FptrMapInt16Uint8     *map[int16]uint8
	FMapInt16Uint16       map[int16]uint16
	FptrMapInt16Uint16    *map[int16]uint16
	FMapInt16Uint32       map[int16]uint32
	FptrMapInt16Uint32    *map[int16]uint32
	FMapInt16Uint64       map[int16]uint64
	FptrMapInt16Uint64    *map[int16]uint64
	FMapInt16Uintptr      map[int16]uintptr
	FptrMapInt16Uintptr   *map[int16]uintptr
	FMapInt16Int          map[int16]int
	FptrMapInt16Int       *map[int16]int
	FMapInt16Int8         map[int16]int8
	FptrMapInt16Int8      *map[int16]int8
	FMapInt16Int16        map[int16]int16
	FptrMapInt16Int16     *map[int16]int16
	FMapInt16Int32        map[int16]int32
	FptrMapInt16Int32     *map[int16]int32
	FMapInt16Int64        map[int16]int64
	FptrMapInt16Int64     *map[int16]int64
	FMapInt16Float32      map[int16]float32
	FptrMapInt16Float32   *map[int16]float32
	FMapInt16Float64      map[int16]float64
	FptrMapInt16Float64   *map[int16]float64
	FMapInt16Bool         map[int16]bool
	FptrMapInt16Bool      *map[int16]bool
	FMapInt32Intf         map[int32]interface{}
	FptrMapInt32Intf      *map[int32]interface{}
	FMapInt32String       map[int32]string
	FptrMapInt32String    *map[int32]string
	FMapInt32Uint         map[int32]uint
	FptrMapInt32Uint      *map[int32]uint
	FMapInt32Uint8        map[int32]uint8
	FptrMapInt32Uint8     *map[int32]uint8
	FMapInt32Uint16       map[int32]uint16
	FptrMapInt32Uint16    *map[int32]uint16
	FMapInt32Uint32       map[int32]uint32
	FptrMapInt32Uint32    *map[int32]uint32
	FMapInt32Uint64       map[int32]uint64
	FptrMapInt32Uint64    *map[int32]uint64
	FMapInt32Uintptr      map[int32]uintptr
	FptrMapInt32Uintptr   *map[int32]uintptr
	FMapInt32Int          map[int32]int
	FptrMapInt32Int       *map[int32]int
	FMapInt32Int8         map[int32]int8
	FptrMapInt32Int8      *map[int32]int8
	FMapInt32Int16        map[int32]int16
	FptrMapInt32Int16     *map[int32]int16
	FMapInt32Int32        map[int32]int32
	FptrMapInt32Int32     *map[int32]int32
	FMapInt32Int64        map[int32]int64
	FptrMapInt32Int64     *map[int32]int64
	FMapInt32Float32      map[int32]float32
	FptrMapInt32Float32   *map[int32]float32
	FMapInt32Float64      map[int32]float64
	FptrMapInt32Float64   *map[int32]float64
	FMapInt32Bool         map[int32]bool
	FptrMapInt32Bool      *map[int32]bool
	FMapInt64Intf         map[int64]interface{}
	FptrMapInt64Intf      *map[int64]interface{}
	FMapInt64String       map[int64]string
	FptrMapInt64String    *map[int64]string
	FMapInt64Uint         map[int64]uint
	FptrMapInt64Uint      *map[int64]uint
	FMapInt64Uint8        map[int64]uint8
	FptrMapInt64Uint8     *map[int64]uint8
	FMapInt64Uint16       map[int64]uint16
	FptrMapInt64Uint16    *map[int64]uint16
	FMapInt64Uint32       map[int64]uint32
	FptrMapInt64Uint32    *map[int64]uint32
	FMapInt64Uint64       map[int64]uint64
	FptrMapInt64Uint64    *map[int64]uint64
	FMapInt64Uintptr      map[int64]uintptr
	FptrMapInt64Uintptr   *map[int64]uintptr
	FMapInt64Int          map[int64]int
	FptrMapInt64Int       *map[int64]int
	FMapInt64Int8         map[int64]int8
	FptrMapInt64Int8      *map[int64]int8
	FMapInt64Int16        map[int64]int16
	FptrMapInt64Int16     *map[int64]int16
	FMapInt64Int32        map[int64]int32
	FptrMapInt64Int32     *map[int64]int32
	FMapInt64Int64        map[int64]int64
	FptrMapInt64Int64     *map[int64]int64
	FMapInt64Float32      map[int64]float32
	FptrMapInt64Float32   *map[int64]float32
	FMapInt64Float64      map[int64]float64
	FptrMapInt64Float64   *map[int64]float64
	FMapInt64Bool         map[int64]bool
	FptrMapInt64Bool      *map[int64]bool
	FMapBoolIntf          map[bool]interface{}
	FptrMapBoolIntf       *map[bool]interface{}
	FMapBoolString        map[bool]string
	FptrMapBoolString     *map[bool]string
	FMapBoolUint          map[bool]uint
	FptrMapBoolUint       *map[bool]uint
	FMapBoolUint8         map[bool]uint8
	FptrMapBoolUint8      *map[bool]uint8
	FMapBoolUint16        map[bool]uint16
	FptrMapBoolUint16     *map[bool]uint16
	FMapBoolUint32        map[bool]uint32
	FptrMapBoolUint32     *map[bool]uint32
	FMapBoolUint64        map[bool]uint64
	FptrMapBoolUint64     *map[bool]uint64
	FMapBoolUintptr       map[bool]uintptr
	FptrMapBoolUintptr    *map[bool]uintptr
	FMapBoolInt           map[bool]int
	FptrMapBoolInt        *map[bool]int
	FMapBoolInt8          map[bool]int8
	FptrMapBoolInt8       *map[bool]int8
	FMapBoolInt16         map[bool]int16
	FptrMapBoolInt16      *map[bool]int16
	FMapBoolInt32         map[bool]int32
	FptrMapBoolInt32      *map[bool]int32
	FMapBoolInt64         map[bool]int64
	FptrMapBoolInt64      *map[bool]int64
	FMapBoolFloat32       map[bool]float32
	FptrMapBoolFloat32    *map[bool]float32
	FMapBoolFloat64       map[bool]float64
	FptrMapBoolFloat64    *map[bool]float64
	FMapBoolBool          map[bool]bool
	FptrMapBoolBool       *map[bool]bool
}

type typeSliceIntf []interface{}

func (_ typeSliceIntf) MapBySlice() {}

type typeSliceString []string

func (_ typeSliceString) MapBySlice() {}

type typeSliceFloat32 []float32

func (_ typeSliceFloat32) MapBySlice() {}

type typeSliceFloat64 []float64

func (_ typeSliceFloat64) MapBySlice() {}

type typeSliceUint []uint

func (_ typeSliceUint) MapBySlice() {}

type typeSliceUint16 []uint16

func (_ typeSliceUint16) MapBySlice() {}

type typeSliceUint32 []uint32

func (_ typeSliceUint32) MapBySlice() {}

type typeSliceUint64 []uint64

func (_ typeSliceUint64) MapBySlice() {}

type typeSliceUintptr []uintptr

func (_ typeSliceUintptr) MapBySlice() {}

type typeSliceInt []int

func (_ typeSliceInt) MapBySlice() {}

type typeSliceInt8 []int8

func (_ typeSliceInt8) MapBySlice() {}

type typeSliceInt16 []int16

func (_ typeSliceInt16) MapBySlice() {}

type typeSliceInt32 []int32

func (_ typeSliceInt32) MapBySlice() {}

type typeSliceInt64 []int64

func (_ typeSliceInt64) MapBySlice() {}

type typeSliceBool []bool

func (_ typeSliceBool) MapBySlice() {}

func doTestMammothSlices(t *testing.T, h Handle) {

	v1v1 := []interface{}{"string-is-an-interface", "string-is-an-interface"}
	bs1, _ := testMarshalErr(v1v1, h, t, "-")
	v1v2 := make([]interface{}, 2)
	testUnmarshalErr(v1v2, bs1, h, t, "-")
	testDeepEqualErr(v1v1, v1v2, t, "-")
	bs1, _ = testMarshalErr(&v1v1, h, t, "-")
	v1v2 = nil
	testUnmarshalErr(&v1v2, bs1, h, t, "-")
	testDeepEqualErr(v1v1, v1v2, t, "-")
	// ...
	v1v2 = make([]interface{}, 2)
	v1v3 := typeSliceIntf(v1v1)
	bs1, _ = testMarshalErr(v1v3, h, t, "-")
	v1v4 := typeSliceIntf(v1v2)
	testUnmarshalErr(v1v4, bs1, h, t, "-")
	testDeepEqualErr(v1v3, v1v4, t, "-")
	v1v2 = nil
	bs1, _ = testMarshalErr(&v1v3, h, t, "-")
	v1v4 = typeSliceIntf(v1v2)
	testUnmarshalErr(&v1v4, bs1, h, t, "-")
	testDeepEqualErr(v1v3, v1v4, t, "-")

	v19v1 := []string{"some-string", "some-string"}
	bs19, _ := testMarshalErr(v19v1, h, t, "-")
	v19v2 := make([]string, 2)
	testUnmarshalErr(v19v2, bs19, h, t, "-")
	testDeepEqualErr(v19v1, v19v2, t, "-")
	bs19, _ = testMarshalErr(&v19v1, h, t, "-")
	v19v2 = nil
	testUnmarshalErr(&v19v2, bs19, h, t, "-")
	testDeepEqualErr(v19v1, v19v2, t, "-")
	// ...
	v19v2 = make([]string, 2)
	v19v3 := typeSliceString(v19v1)
	bs19, _ = testMarshalErr(v19v3, h, t, "-")
	v19v4 := typeSliceString(v19v2)
	testUnmarshalErr(v19v4, bs19, h, t, "-")
	testDeepEqualErr(v19v3, v19v4, t, "-")
	v19v2 = nil
	bs19, _ = testMarshalErr(&v19v3, h, t, "-")
	v19v4 = typeSliceString(v19v2)
	testUnmarshalErr(&v19v4, bs19, h, t, "-")
	testDeepEqualErr(v19v3, v19v4, t, "-")

	v37v1 := []float32{10.1, 10.1}
	bs37, _ := testMarshalErr(v37v1, h, t, "-")
	v37v2 := make([]float32, 2)
	testUnmarshalErr(v37v2, bs37, h, t, "-")
	testDeepEqualErr(v37v1, v37v2, t, "-")
	bs37, _ = testMarshalErr(&v37v1, h, t, "-")
	v37v2 = nil
	testUnmarshalErr(&v37v2, bs37, h, t, "-")
	testDeepEqualErr(v37v1, v37v2, t, "-")
	// ...
	v37v2 = make([]float32, 2)
	v37v3 := typeSliceFloat32(v37v1)
	bs37, _ = testMarshalErr(v37v3, h, t, "-")
	v37v4 := typeSliceFloat32(v37v2)
	testUnmarshalErr(v37v4, bs37, h, t, "-")
	testDeepEqualErr(v37v3, v37v4, t, "-")
	v37v2 = nil
	bs37, _ = testMarshalErr(&v37v3, h, t, "-")
	v37v4 = typeSliceFloat32(v37v2)
	testUnmarshalErr(&v37v4, bs37, h, t, "-")
	testDeepEqualErr(v37v3, v37v4, t, "-")

	v55v1 := []float64{10.1, 10.1}
	bs55, _ := testMarshalErr(v55v1, h, t, "-")
	v55v2 := make([]float64, 2)
	testUnmarshalErr(v55v2, bs55, h, t, "-")
	testDeepEqualErr(v55v1, v55v2, t, "-")
	bs55, _ = testMarshalErr(&v55v1, h, t, "-")
	v55v2 = nil
	testUnmarshalErr(&v55v2, bs55, h, t, "-")
	testDeepEqualErr(v55v1, v55v2, t, "-")
	// ...
	v55v2 = make([]float64, 2)
	v55v3 := typeSliceFloat64(v55v1)
	bs55, _ = testMarshalErr(v55v3, h, t, "-")
	v55v4 := typeSliceFloat64(v55v2)
	testUnmarshalErr(v55v4, bs55, h, t, "-")
	testDeepEqualErr(v55v3, v55v4, t, "-")
	v55v2 = nil
	bs55, _ = testMarshalErr(&v55v3, h, t, "-")
	v55v4 = typeSliceFloat64(v55v2)
	testUnmarshalErr(&v55v4, bs55, h, t, "-")
	testDeepEqualErr(v55v3, v55v4, t, "-")

	v73v1 := []uint{10, 10}
	bs73, _ := testMarshalErr(v73v1, h, t, "-")
	v73v2 := make([]uint, 2)
	testUnmarshalErr(v73v2, bs73, h, t, "-")
	testDeepEqualErr(v73v1, v73v2, t, "-")
	bs73, _ = testMarshalErr(&v73v1, h, t, "-")
	v73v2 = nil
	testUnmarshalErr(&v73v2, bs73, h, t, "-")
	testDeepEqualErr(v73v1, v73v2, t, "-")
	// ...
	v73v2 = make([]uint, 2)
	v73v3 := typeSliceUint(v73v1)
	bs73, _ = testMarshalErr(v73v3, h, t, "-")
	v73v4 := typeSliceUint(v73v2)
	testUnmarshalErr(v73v4, bs73, h, t, "-")
	testDeepEqualErr(v73v3, v73v4, t, "-")
	v73v2 = nil
	bs73, _ = testMarshalErr(&v73v3, h, t, "-")
	v73v4 = typeSliceUint(v73v2)
	testUnmarshalErr(&v73v4, bs73, h, t, "-")
	testDeepEqualErr(v73v3, v73v4, t, "-")

	v108v1 := []uint16{10, 10}
	bs108, _ := testMarshalErr(v108v1, h, t, "-")
	v108v2 := make([]uint16, 2)
	testUnmarshalErr(v108v2, bs108, h, t, "-")
	testDeepEqualErr(v108v1, v108v2, t, "-")
	bs108, _ = testMarshalErr(&v108v1, h, t, "-")
	v108v2 = nil
	testUnmarshalErr(&v108v2, bs108, h, t, "-")
	testDeepEqualErr(v108v1, v108v2, t, "-")
	// ...
	v108v2 = make([]uint16, 2)
	v108v3 := typeSliceUint16(v108v1)
	bs108, _ = testMarshalErr(v108v3, h, t, "-")
	v108v4 := typeSliceUint16(v108v2)
	testUnmarshalErr(v108v4, bs108, h, t, "-")
	testDeepEqualErr(v108v3, v108v4, t, "-")
	v108v2 = nil
	bs108, _ = testMarshalErr(&v108v3, h, t, "-")
	v108v4 = typeSliceUint16(v108v2)
	testUnmarshalErr(&v108v4, bs108, h, t, "-")
	testDeepEqualErr(v108v3, v108v4, t, "-")

	v126v1 := []uint32{10, 10}
	bs126, _ := testMarshalErr(v126v1, h, t, "-")
	v126v2 := make([]uint32, 2)
	testUnmarshalErr(v126v2, bs126, h, t, "-")
	testDeepEqualErr(v126v1, v126v2, t, "-")
	bs126, _ = testMarshalErr(&v126v1, h, t, "-")
	v126v2 = nil
	testUnmarshalErr(&v126v2, bs126, h, t, "-")
	testDeepEqualErr(v126v1, v126v2, t, "-")
	// ...
	v126v2 = make([]uint32, 2)
	v126v3 := typeSliceUint32(v126v1)
	bs126, _ = testMarshalErr(v126v3, h, t, "-")
	v126v4 := typeSliceUint32(v126v2)
	testUnmarshalErr(v126v4, bs126, h, t, "-")
	testDeepEqualErr(v126v3, v126v4, t, "-")
	v126v2 = nil
	bs126, _ = testMarshalErr(&v126v3, h, t, "-")
	v126v4 = typeSliceUint32(v126v2)
	testUnmarshalErr(&v126v4, bs126, h, t, "-")
	testDeepEqualErr(v126v3, v126v4, t, "-")

	v144v1 := []uint64{10, 10}
	bs144, _ := testMarshalErr(v144v1, h, t, "-")
	v144v2 := make([]uint64, 2)
	testUnmarshalErr(v144v2, bs144, h, t, "-")
	testDeepEqualErr(v144v1, v144v2, t, "-")
	bs144, _ = testMarshalErr(&v144v1, h, t, "-")
	v144v2 = nil
	testUnmarshalErr(&v144v2, bs144, h, t, "-")
	testDeepEqualErr(v144v1, v144v2, t, "-")
	// ...
	v144v2 = make([]uint64, 2)
	v144v3 := typeSliceUint64(v144v1)
	bs144, _ = testMarshalErr(v144v3, h, t, "-")
	v144v4 := typeSliceUint64(v144v2)
	testUnmarshalErr(v144v4, bs144, h, t, "-")
	testDeepEqualErr(v144v3, v144v4, t, "-")
	v144v2 = nil
	bs144, _ = testMarshalErr(&v144v3, h, t, "-")
	v144v4 = typeSliceUint64(v144v2)
	testUnmarshalErr(&v144v4, bs144, h, t, "-")
	testDeepEqualErr(v144v3, v144v4, t, "-")

	v162v1 := []uintptr{10, 10}
	bs162, _ := testMarshalErr(v162v1, h, t, "-")
	v162v2 := make([]uintptr, 2)
	testUnmarshalErr(v162v2, bs162, h, t, "-")
	testDeepEqualErr(v162v1, v162v2, t, "-")
	bs162, _ = testMarshalErr(&v162v1, h, t, "-")
	v162v2 = nil
	testUnmarshalErr(&v162v2, bs162, h, t, "-")
	testDeepEqualErr(v162v1, v162v2, t, "-")
	// ...
	v162v2 = make([]uintptr, 2)
	v162v3 := typeSliceUintptr(v162v1)
	bs162, _ = testMarshalErr(v162v3, h, t, "-")
	v162v4 := typeSliceUintptr(v162v2)
	testUnmarshalErr(v162v4, bs162, h, t, "-")
	testDeepEqualErr(v162v3, v162v4, t, "-")
	v162v2 = nil
	bs162, _ = testMarshalErr(&v162v3, h, t, "-")
	v162v4 = typeSliceUintptr(v162v2)
	testUnmarshalErr(&v162v4, bs162, h, t, "-")
	testDeepEqualErr(v162v3, v162v4, t, "-")

	v180v1 := []int{10, 10}
	bs180, _ := testMarshalErr(v180v1, h, t, "-")
	v180v2 := make([]int, 2)
	testUnmarshalErr(v180v2, bs180, h, t, "-")
	testDeepEqualErr(v180v1, v180v2, t, "-")
	bs180, _ = testMarshalErr(&v180v1, h, t, "-")
	v180v2 = nil
	testUnmarshalErr(&v180v2, bs180, h, t, "-")
	testDeepEqualErr(v180v1, v180v2, t, "-")
	// ...
	v180v2 = make([]int, 2)
	v180v3 := typeSliceInt(v180v1)
	bs180, _ = testMarshalErr(v180v3, h, t, "-")
	v180v4 := typeSliceInt(v180v2)
	testUnmarshalErr(v180v4, bs180, h, t, "-")
	testDeepEqualErr(v180v3, v180v4, t, "-")
	v180v2 = nil
	bs180, _ = testMarshalErr(&v180v3, h, t, "-")
	v180v4 = typeSliceInt(v180v2)
	testUnmarshalErr(&v180v4, bs180, h, t, "-")
	testDeepEqualErr(v180v3, v180v4, t, "-")

	v198v1 := []int8{10, 10}
	bs198, _ := testMarshalErr(v198v1, h, t, "-")
	v198v2 := make([]int8, 2)
	testUnmarshalErr(v198v2, bs198, h, t, "-")
	testDeepEqualErr(v198v1, v198v2, t, "-")
	bs198, _ = testMarshalErr(&v198v1, h, t, "-")
	v198v2 = nil
	testUnmarshalErr(&v198v2, bs198, h, t, "-")
	testDeepEqualErr(v198v1, v198v2, t, "-")
	// ...
	v198v2 = make([]int8, 2)
	v198v3 := typeSliceInt8(v198v1)
	bs198, _ = testMarshalErr(v198v3, h, t, "-")
	v198v4 := typeSliceInt8(v198v2)
	testUnmarshalErr(v198v4, bs198, h, t, "-")
	testDeepEqualErr(v198v3, v198v4, t, "-")
	v198v2 = nil
	bs198, _ = testMarshalErr(&v198v3, h, t, "-")
	v198v4 = typeSliceInt8(v198v2)
	testUnmarshalErr(&v198v4, bs198, h, t, "-")
	testDeepEqualErr(v198v3, v198v4, t, "-")

	v216v1 := []int16{10, 10}
	bs216, _ := testMarshalErr(v216v1, h, t, "-")
	v216v2 := make([]int16, 2)
	testUnmarshalErr(v216v2, bs216, h, t, "-")
	testDeepEqualErr(v216v1, v216v2, t, "-")
	bs216, _ = testMarshalErr(&v216v1, h, t, "-")
	v216v2 = nil
	testUnmarshalErr(&v216v2, bs216, h, t, "-")
	testDeepEqualErr(v216v1, v216v2, t, "-")
	// ...
	v216v2 = make([]int16, 2)
	v216v3 := typeSliceInt16(v216v1)
	bs216, _ = testMarshalErr(v216v3, h, t, "-")
	v216v4 := typeSliceInt16(v216v2)
	testUnmarshalErr(v216v4, bs216, h, t, "-")
	testDeepEqualErr(v216v3, v216v4, t, "-")
	v216v2 = nil
	bs216, _ = testMarshalErr(&v216v3, h, t, "-")
	v216v4 = typeSliceInt16(v216v2)
	testUnmarshalErr(&v216v4, bs216, h, t, "-")
	testDeepEqualErr(v216v3, v216v4, t, "-")

	v234v1 := []int32{10, 10}
	bs234, _ := testMarshalErr(v234v1, h, t, "-")
	v234v2 := make([]int32, 2)
	testUnmarshalErr(v234v2, bs234, h, t, "-")
	testDeepEqualErr(v234v1, v234v2, t, "-")
	bs234, _ = testMarshalErr(&v234v1, h, t, "-")
	v234v2 = nil
	testUnmarshalErr(&v234v2, bs234, h, t, "-")
	testDeepEqualErr(v234v1, v234v2, t, "-")
	// ...
	v234v2 = make([]int32, 2)
	v234v3 := typeSliceInt32(v234v1)
	bs234, _ = testMarshalErr(v234v3, h, t, "-")
	v234v4 := typeSliceInt32(v234v2)
	testUnmarshalErr(v234v4, bs234, h, t, "-")
	testDeepEqualErr(v234v3, v234v4, t, "-")
	v234v2 = nil
	bs234, _ = testMarshalErr(&v234v3, h, t, "-")
	v234v4 = typeSliceInt32(v234v2)
	testUnmarshalErr(&v234v4, bs234, h, t, "-")
	testDeepEqualErr(v234v3, v234v4, t, "-")

	v252v1 := []int64{10, 10}
	bs252, _ := testMarshalErr(v252v1, h, t, "-")
	v252v2 := make([]int64, 2)
	testUnmarshalErr(v252v2, bs252, h, t, "-")
	testDeepEqualErr(v252v1, v252v2, t, "-")
	bs252, _ = testMarshalErr(&v252v1, h, t, "-")
	v252v2 = nil
	testUnmarshalErr(&v252v2, bs252, h, t, "-")
	testDeepEqualErr(v252v1, v252v2, t, "-")
	// ...
	v252v2 = make([]int64, 2)
	v252v3 := typeSliceInt64(v252v1)
	bs252, _ = testMarshalErr(v252v3, h, t, "-")
	v252v4 := typeSliceInt64(v252v2)
	testUnmarshalErr(v252v4, bs252, h, t, "-")
	testDeepEqualErr(v252v3, v252v4, t, "-")
	v252v2 = nil
	bs252, _ = testMarshalErr(&v252v3, h, t, "-")
	v252v4 = typeSliceInt64(v252v2)
	testUnmarshalErr(&v252v4, bs252, h, t, "-")
	testDeepEqualErr(v252v3, v252v4, t, "-")

	v270v1 := []bool{true, true}
	bs270, _ := testMarshalErr(v270v1, h, t, "-")
	v270v2 := make([]bool, 2)
	testUnmarshalErr(v270v2, bs270, h, t, "-")
	testDeepEqualErr(v270v1, v270v2, t, "-")
	bs270, _ = testMarshalErr(&v270v1, h, t, "-")
	v270v2 = nil
	testUnmarshalErr(&v270v2, bs270, h, t, "-")
	testDeepEqualErr(v270v1, v270v2, t, "-")
	// ...
	v270v2 = make([]bool, 2)
	v270v3 := typeSliceBool(v270v1)
	bs270, _ = testMarshalErr(v270v3, h, t, "-")
	v270v4 := typeSliceBool(v270v2)
	testUnmarshalErr(v270v4, bs270, h, t, "-")
	testDeepEqualErr(v270v3, v270v4, t, "-")
	v270v2 = nil
	bs270, _ = testMarshalErr(&v270v3, h, t, "-")
	v270v4 = typeSliceBool(v270v2)
	testUnmarshalErr(&v270v4, bs270, h, t, "-")
	testDeepEqualErr(v270v3, v270v4, t, "-")

}

func doTestMammothMaps(t *testing.T, h Handle) {

	v2v1 := map[interface{}]interface{}{"string-is-an-interface": "string-is-an-interface"}
	bs2, _ := testMarshalErr(v2v1, h, t, "-")
	v2v2 := make(map[interface{}]interface{})
	testUnmarshalErr(v2v2, bs2, h, t, "-")
	testDeepEqualErr(v2v1, v2v2, t, "-")
	bs2, _ = testMarshalErr(&v2v1, h, t, "-")
	v2v2 = nil
	testUnmarshalErr(&v2v2, bs2, h, t, "-")
	testDeepEqualErr(v2v1, v2v2, t, "-")

	v3v1 := map[interface{}]string{"string-is-an-interface": "some-string"}
	bs3, _ := testMarshalErr(v3v1, h, t, "-")
	v3v2 := make(map[interface{}]string)
	testUnmarshalErr(v3v2, bs3, h, t, "-")
	testDeepEqualErr(v3v1, v3v2, t, "-")
	bs3, _ = testMarshalErr(&v3v1, h, t, "-")
	v3v2 = nil
	testUnmarshalErr(&v3v2, bs3, h, t, "-")
	testDeepEqualErr(v3v1, v3v2, t, "-")

	v4v1 := map[interface{}]uint{"string-is-an-interface": 10}
	bs4, _ := testMarshalErr(v4v1, h, t, "-")
	v4v2 := make(map[interface{}]uint)
	testUnmarshalErr(v4v2, bs4, h, t, "-")
	testDeepEqualErr(v4v1, v4v2, t, "-")
	bs4, _ = testMarshalErr(&v4v1, h, t, "-")
	v4v2 = nil
	testUnmarshalErr(&v4v2, bs4, h, t, "-")
	testDeepEqualErr(v4v1, v4v2, t, "-")

	v5v1 := map[interface{}]uint8{"string-is-an-interface": 10}
	bs5, _ := testMarshalErr(v5v1, h, t, "-")
	v5v2 := make(map[interface{}]uint8)
	testUnmarshalErr(v5v2, bs5, h, t, "-")
	testDeepEqualErr(v5v1, v5v2, t, "-")
	bs5, _ = testMarshalErr(&v5v1, h, t, "-")
	v5v2 = nil
	testUnmarshalErr(&v5v2, bs5, h, t, "-")
	testDeepEqualErr(v5v1, v5v2, t, "-")

	v6v1 := map[interface{}]uint16{"string-is-an-interface": 10}
	bs6, _ := testMarshalErr(v6v1, h, t, "-")
	v6v2 := make(map[interface{}]uint16)
	testUnmarshalErr(v6v2, bs6, h, t, "-")
	testDeepEqualErr(v6v1, v6v2, t, "-")
	bs6, _ = testMarshalErr(&v6v1, h, t, "-")
	v6v2 = nil
	testUnmarshalErr(&v6v2, bs6, h, t, "-")
	testDeepEqualErr(v6v1, v6v2, t, "-")

	v7v1 := map[interface{}]uint32{"string-is-an-interface": 10}
	bs7, _ := testMarshalErr(v7v1, h, t, "-")
	v7v2 := make(map[interface{}]uint32)
	testUnmarshalErr(v7v2, bs7, h, t, "-")
	testDeepEqualErr(v7v1, v7v2, t, "-")
	bs7, _ = testMarshalErr(&v7v1, h, t, "-")
	v7v2 = nil
	testUnmarshalErr(&v7v2, bs7, h, t, "-")
	testDeepEqualErr(v7v1, v7v2, t, "-")

	v8v1 := map[interface{}]uint64{"string-is-an-interface": 10}
	bs8, _ := testMarshalErr(v8v1, h, t, "-")
	v8v2 := make(map[interface{}]uint64)
	testUnmarshalErr(v8v2, bs8, h, t, "-")
	testDeepEqualErr(v8v1, v8v2, t, "-")
	bs8, _ = testMarshalErr(&v8v1, h, t, "-")
	v8v2 = nil
	testUnmarshalErr(&v8v2, bs8, h, t, "-")
	testDeepEqualErr(v8v1, v8v2, t, "-")

	v9v1 := map[interface{}]uintptr{"string-is-an-interface": 10}
	bs9, _ := testMarshalErr(v9v1, h, t, "-")
	v9v2 := make(map[interface{}]uintptr)
	testUnmarshalErr(v9v2, bs9, h, t, "-")
	testDeepEqualErr(v9v1, v9v2, t, "-")
	bs9, _ = testMarshalErr(&v9v1, h, t, "-")
	v9v2 = nil
	testUnmarshalErr(&v9v2, bs9, h, t, "-")
	testDeepEqualErr(v9v1, v9v2, t, "-")

	v10v1 := map[interface{}]int{"string-is-an-interface": 10}
	bs10, _ := testMarshalErr(v10v1, h, t, "-")
	v10v2 := make(map[interface{}]int)
	testUnmarshalErr(v10v2, bs10, h, t, "-")
	testDeepEqualErr(v10v1, v10v2, t, "-")
	bs10, _ = testMarshalErr(&v10v1, h, t, "-")
	v10v2 = nil
	testUnmarshalErr(&v10v2, bs10, h, t, "-")
	testDeepEqualErr(v10v1, v10v2, t, "-")

	v11v1 := map[interface{}]int8{"string-is-an-interface": 10}
	bs11, _ := testMarshalErr(v11v1, h, t, "-")
	v11v2 := make(map[interface{}]int8)
	testUnmarshalErr(v11v2, bs11, h, t, "-")
	testDeepEqualErr(v11v1, v11v2, t, "-")
	bs11, _ = testMarshalErr(&v11v1, h, t, "-")
	v11v2 = nil
	testUnmarshalErr(&v11v2, bs11, h, t, "-")
	testDeepEqualErr(v11v1, v11v2, t, "-")

	v12v1 := map[interface{}]int16{"string-is-an-interface": 10}
	bs12, _ := testMarshalErr(v12v1, h, t, "-")
	v12v2 := make(map[interface{}]int16)
	testUnmarshalErr(v12v2, bs12, h, t, "-")
	testDeepEqualErr(v12v1, v12v2, t, "-")
	bs12, _ = testMarshalErr(&v12v1, h, t, "-")
	v12v2 = nil
	testUnmarshalErr(&v12v2, bs12, h, t, "-")
	testDeepEqualErr(v12v1, v12v2, t, "-")

	v13v1 := map[interface{}]int32{"string-is-an-interface": 10}
	bs13, _ := testMarshalErr(v13v1, h, t, "-")
	v13v2 := make(map[interface{}]int32)
	testUnmarshalErr(v13v2, bs13, h, t, "-")
	testDeepEqualErr(v13v1, v13v2, t, "-")
	bs13, _ = testMarshalErr(&v13v1, h, t, "-")
	v13v2 = nil
	testUnmarshalErr(&v13v2, bs13, h, t, "-")
	testDeepEqualErr(v13v1, v13v2, t, "-")

	v14v1 := map[interface{}]int64{"string-is-an-interface": 10}
	bs14, _ := testMarshalErr(v14v1, h, t, "-")
	v14v2 := make(map[interface{}]int64)
	testUnmarshalErr(v14v2, bs14, h, t, "-")
	testDeepEqualErr(v14v1, v14v2, t, "-")
	bs14, _ = testMarshalErr(&v14v1, h, t, "-")
	v14v2 = nil
	testUnmarshalErr(&v14v2, bs14, h, t, "-")
	testDeepEqualErr(v14v1, v14v2, t, "-")

	v15v1 := map[interface{}]float32{"string-is-an-interface": 10.1}
	bs15, _ := testMarshalErr(v15v1, h, t, "-")
	v15v2 := make(map[interface{}]float32)
	testUnmarshalErr(v15v2, bs15, h, t, "-")
	testDeepEqualErr(v15v1, v15v2, t, "-")
	bs15, _ = testMarshalErr(&v15v1, h, t, "-")
	v15v2 = nil
	testUnmarshalErr(&v15v2, bs15, h, t, "-")
	testDeepEqualErr(v15v1, v15v2, t, "-")

	v16v1 := map[interface{}]float64{"string-is-an-interface": 10.1}
	bs16, _ := testMarshalErr(v16v1, h, t, "-")
	v16v2 := make(map[interface{}]float64)
	testUnmarshalErr(v16v2, bs16, h, t, "-")
	testDeepEqualErr(v16v1, v16v2, t, "-")
	bs16, _ = testMarshalErr(&v16v1, h, t, "-")
	v16v2 = nil
	testUnmarshalErr(&v16v2, bs16, h, t, "-")
	testDeepEqualErr(v16v1, v16v2, t, "-")

	v17v1 := map[interface{}]bool{"string-is-an-interface": true}
	bs17, _ := testMarshalErr(v17v1, h, t, "-")
	v17v2 := make(map[interface{}]bool)
	testUnmarshalErr(v17v2, bs17, h, t, "-")
	testDeepEqualErr(v17v1, v17v2, t, "-")
	bs17, _ = testMarshalErr(&v17v1, h, t, "-")
	v17v2 = nil
	testUnmarshalErr(&v17v2, bs17, h, t, "-")
	testDeepEqualErr(v17v1, v17v2, t, "-")

	v20v1 := map[string]interface{}{"some-string": "string-is-an-interface"}
	bs20, _ := testMarshalErr(v20v1, h, t, "-")
	v20v2 := make(map[string]interface{})
	testUnmarshalErr(v20v2, bs20, h, t, "-")
	testDeepEqualErr(v20v1, v20v2, t, "-")
	bs20, _ = testMarshalErr(&v20v1, h, t, "-")
	v20v2 = nil
	testUnmarshalErr(&v20v2, bs20, h, t, "-")
	testDeepEqualErr(v20v1, v20v2, t, "-")

	v21v1 := map[string]string{"some-string": "some-string"}
	bs21, _ := testMarshalErr(v21v1, h, t, "-")
	v21v2 := make(map[string]string)
	testUnmarshalErr(v21v2, bs21, h, t, "-")
	testDeepEqualErr(v21v1, v21v2, t, "-")
	bs21, _ = testMarshalErr(&v21v1, h, t, "-")
	v21v2 = nil
	testUnmarshalErr(&v21v2, bs21, h, t, "-")
	testDeepEqualErr(v21v1, v21v2, t, "-")

	v22v1 := map[string]uint{"some-string": 10}
	bs22, _ := testMarshalErr(v22v1, h, t, "-")
	v22v2 := make(map[string]uint)
	testUnmarshalErr(v22v2, bs22, h, t, "-")
	testDeepEqualErr(v22v1, v22v2, t, "-")
	bs22, _ = testMarshalErr(&v22v1, h, t, "-")
	v22v2 = nil
	testUnmarshalErr(&v22v2, bs22, h, t, "-")
	testDeepEqualErr(v22v1, v22v2, t, "-")

	v23v1 := map[string]uint8{"some-string": 10}
	bs23, _ := testMarshalErr(v23v1, h, t, "-")
	v23v2 := make(map[string]uint8)
	testUnmarshalErr(v23v2, bs23, h, t, "-")
	testDeepEqualErr(v23v1, v23v2, t, "-")
	bs23, _ = testMarshalErr(&v23v1, h, t, "-")
	v23v2 = nil
	testUnmarshalErr(&v23v2, bs23, h, t, "-")
	testDeepEqualErr(v23v1, v23v2, t, "-")

	v24v1 := map[string]uint16{"some-string": 10}
	bs24, _ := testMarshalErr(v24v1, h, t, "-")
	v24v2 := make(map[string]uint16)
	testUnmarshalErr(v24v2, bs24, h, t, "-")
	testDeepEqualErr(v24v1, v24v2, t, "-")
	bs24, _ = testMarshalErr(&v24v1, h, t, "-")
	v24v2 = nil
	testUnmarshalErr(&v24v2, bs24, h, t, "-")
	testDeepEqualErr(v24v1, v24v2, t, "-")

	v25v1 := map[string]uint32{"some-string": 10}
	bs25, _ := testMarshalErr(v25v1, h, t, "-")
	v25v2 := make(map[string]uint32)
	testUnmarshalErr(v25v2, bs25, h, t, "-")
	testDeepEqualErr(v25v1, v25v2, t, "-")
	bs25, _ = testMarshalErr(&v25v1, h, t, "-")
	v25v2 = nil
	testUnmarshalErr(&v25v2, bs25, h, t, "-")
	testDeepEqualErr(v25v1, v25v2, t, "-")

	v26v1 := map[string]uint64{"some-string": 10}
	bs26, _ := testMarshalErr(v26v1, h, t, "-")
	v26v2 := make(map[string]uint64)
	testUnmarshalErr(v26v2, bs26, h, t, "-")
	testDeepEqualErr(v26v1, v26v2, t, "-")
	bs26, _ = testMarshalErr(&v26v1, h, t, "-")
	v26v2 = nil
	testUnmarshalErr(&v26v2, bs26, h, t, "-")
	testDeepEqualErr(v26v1, v26v2, t, "-")

	v27v1 := map[string]uintptr{"some-string": 10}
	bs27, _ := testMarshalErr(v27v1, h, t, "-")
	v27v2 := make(map[string]uintptr)
	testUnmarshalErr(v27v2, bs27, h, t, "-")
	testDeepEqualErr(v27v1, v27v2, t, "-")
	bs27, _ = testMarshalErr(&v27v1, h, t, "-")
	v27v2 = nil
	testUnmarshalErr(&v27v2, bs27, h, t, "-")
	testDeepEqualErr(v27v1, v27v2, t, "-")

	v28v1 := map[string]int{"some-string": 10}
	bs28, _ := testMarshalErr(v28v1, h, t, "-")
	v28v2 := make(map[string]int)
	testUnmarshalErr(v28v2, bs28, h, t, "-")
	testDeepEqualErr(v28v1, v28v2, t, "-")
	bs28, _ = testMarshalErr(&v28v1, h, t, "-")
	v28v2 = nil
	testUnmarshalErr(&v28v2, bs28, h, t, "-")
	testDeepEqualErr(v28v1, v28v2, t, "-")

	v29v1 := map[string]int8{"some-string": 10}
	bs29, _ := testMarshalErr(v29v1, h, t, "-")
	v29v2 := make(map[string]int8)
	testUnmarshalErr(v29v2, bs29, h, t, "-")
	testDeepEqualErr(v29v1, v29v2, t, "-")
	bs29, _ = testMarshalErr(&v29v1, h, t, "-")
	v29v2 = nil
	testUnmarshalErr(&v29v2, bs29, h, t, "-")
	testDeepEqualErr(v29v1, v29v2, t, "-")

	v30v1 := map[string]int16{"some-string": 10}
	bs30, _ := testMarshalErr(v30v1, h, t, "-")
	v30v2 := make(map[string]int16)
	testUnmarshalErr(v30v2, bs30, h, t, "-")
	testDeepEqualErr(v30v1, v30v2, t, "-")
	bs30, _ = testMarshalErr(&v30v1, h, t, "-")
	v30v2 = nil
	testUnmarshalErr(&v30v2, bs30, h, t, "-")
	testDeepEqualErr(v30v1, v30v2, t, "-")

	v31v1 := map[string]int32{"some-string": 10}
	bs31, _ := testMarshalErr(v31v1, h, t, "-")
	v31v2 := make(map[string]int32)
	testUnmarshalErr(v31v2, bs31, h, t, "-")
	testDeepEqualErr(v31v1, v31v2, t, "-")
	bs31, _ = testMarshalErr(&v31v1, h, t, "-")
	v31v2 = nil
	testUnmarshalErr(&v31v2, bs31, h, t, "-")
	testDeepEqualErr(v31v1, v31v2, t, "-")

	v32v1 := map[string]int64{"some-string": 10}
	bs32, _ := testMarshalErr(v32v1, h, t, "-")
	v32v2 := make(map[string]int64)
	testUnmarshalErr(v32v2, bs32, h, t, "-")
	testDeepEqualErr(v32v1, v32v2, t, "-")
	bs32, _ = testMarshalErr(&v32v1, h, t, "-")
	v32v2 = nil
	testUnmarshalErr(&v32v2, bs32, h, t, "-")
	testDeepEqualErr(v32v1, v32v2, t, "-")

	v33v1 := map[string]float32{"some-string": 10.1}
	bs33, _ := testMarshalErr(v33v1, h, t, "-")
	v33v2 := make(map[string]float32)
	testUnmarshalErr(v33v2, bs33, h, t, "-")
	testDeepEqualErr(v33v1, v33v2, t, "-")
	bs33, _ = testMarshalErr(&v33v1, h, t, "-")
	v33v2 = nil
	testUnmarshalErr(&v33v2, bs33, h, t, "-")
	testDeepEqualErr(v33v1, v33v2, t, "-")

	v34v1 := map[string]float64{"some-string": 10.1}
	bs34, _ := testMarshalErr(v34v1, h, t, "-")
	v34v2 := make(map[string]float64)
	testUnmarshalErr(v34v2, bs34, h, t, "-")
	testDeepEqualErr(v34v1, v34v2, t, "-")
	bs34, _ = testMarshalErr(&v34v1, h, t, "-")
	v34v2 = nil
	testUnmarshalErr(&v34v2, bs34, h, t, "-")
	testDeepEqualErr(v34v1, v34v2, t, "-")

	v35v1 := map[string]bool{"some-string": true}
	bs35, _ := testMarshalErr(v35v1, h, t, "-")
	v35v2 := make(map[string]bool)
	testUnmarshalErr(v35v2, bs35, h, t, "-")
	testDeepEqualErr(v35v1, v35v2, t, "-")
	bs35, _ = testMarshalErr(&v35v1, h, t, "-")
	v35v2 = nil
	testUnmarshalErr(&v35v2, bs35, h, t, "-")
	testDeepEqualErr(v35v1, v35v2, t, "-")

	v38v1 := map[float32]interface{}{10.1: "string-is-an-interface"}
	bs38, _ := testMarshalErr(v38v1, h, t, "-")
	v38v2 := make(map[float32]interface{})
	testUnmarshalErr(v38v2, bs38, h, t, "-")
	testDeepEqualErr(v38v1, v38v2, t, "-")
	bs38, _ = testMarshalErr(&v38v1, h, t, "-")
	v38v2 = nil
	testUnmarshalErr(&v38v2, bs38, h, t, "-")
	testDeepEqualErr(v38v1, v38v2, t, "-")

	v39v1 := map[float32]string{10.1: "some-string"}
	bs39, _ := testMarshalErr(v39v1, h, t, "-")
	v39v2 := make(map[float32]string)
	testUnmarshalErr(v39v2, bs39, h, t, "-")
	testDeepEqualErr(v39v1, v39v2, t, "-")
	bs39, _ = testMarshalErr(&v39v1, h, t, "-")
	v39v2 = nil
	testUnmarshalErr(&v39v2, bs39, h, t, "-")
	testDeepEqualErr(v39v1, v39v2, t, "-")

	v40v1 := map[float32]uint{10.1: 10}
	bs40, _ := testMarshalErr(v40v1, h, t, "-")
	v40v2 := make(map[float32]uint)
	testUnmarshalErr(v40v2, bs40, h, t, "-")
	testDeepEqualErr(v40v1, v40v2, t, "-")
	bs40, _ = testMarshalErr(&v40v1, h, t, "-")
	v40v2 = nil
	testUnmarshalErr(&v40v2, bs40, h, t, "-")
	testDeepEqualErr(v40v1, v40v2, t, "-")

	v41v1 := map[float32]uint8{10.1: 10}
	bs41, _ := testMarshalErr(v41v1, h, t, "-")
	v41v2 := make(map[float32]uint8)
	testUnmarshalErr(v41v2, bs41, h, t, "-")
	testDeepEqualErr(v41v1, v41v2, t, "-")
	bs41, _ = testMarshalErr(&v41v1, h, t, "-")
	v41v2 = nil
	testUnmarshalErr(&v41v2, bs41, h, t, "-")
	testDeepEqualErr(v41v1, v41v2, t, "-")

	v42v1 := map[float32]uint16{10.1: 10}
	bs42, _ := testMarshalErr(v42v1, h, t, "-")
	v42v2 := make(map[float32]uint16)
	testUnmarshalErr(v42v2, bs42, h, t, "-")
	testDeepEqualErr(v42v1, v42v2, t, "-")
	bs42, _ = testMarshalErr(&v42v1, h, t, "-")
	v42v2 = nil
	testUnmarshalErr(&v42v2, bs42, h, t, "-")
	testDeepEqualErr(v42v1, v42v2, t, "-")

	v43v1 := map[float32]uint32{10.1: 10}
	bs43, _ := testMarshalErr(v43v1, h, t, "-")
	v43v2 := make(map[float32]uint32)
	testUnmarshalErr(v43v2, bs43, h, t, "-")
	testDeepEqualErr(v43v1, v43v2, t, "-")
	bs43, _ = testMarshalErr(&v43v1, h, t, "-")
	v43v2 = nil
	testUnmarshalErr(&v43v2, bs43, h, t, "-")
	testDeepEqualErr(v43v1, v43v2, t, "-")

	v44v1 := map[float32]uint64{10.1: 10}
	bs44, _ := testMarshalErr(v44v1, h, t, "-")
	v44v2 := make(map[float32]uint64)
	testUnmarshalErr(v44v2, bs44, h, t, "-")
	testDeepEqualErr(v44v1, v44v2, t, "-")
	bs44, _ = testMarshalErr(&v44v1, h, t, "-")
	v44v2 = nil
	testUnmarshalErr(&v44v2, bs44, h, t, "-")
	testDeepEqualErr(v44v1, v44v2, t, "-")

	v45v1 := map[float32]uintptr{10.1: 10}
	bs45, _ := testMarshalErr(v45v1, h, t, "-")
	v45v2 := make(map[float32]uintptr)
	testUnmarshalErr(v45v2, bs45, h, t, "-")
	testDeepEqualErr(v45v1, v45v2, t, "-")
	bs45, _ = testMarshalErr(&v45v1, h, t, "-")
	v45v2 = nil
	testUnmarshalErr(&v45v2, bs45, h, t, "-")
	testDeepEqualErr(v45v1, v45v2, t, "-")

	v46v1 := map[float32]int{10.1: 10}
	bs46, _ := testMarshalErr(v46v1, h, t, "-")
	v46v2 := make(map[float32]int)
	testUnmarshalErr(v46v2, bs46, h, t, "-")
	testDeepEqualErr(v46v1, v46v2, t, "-")
	bs46, _ = testMarshalErr(&v46v1, h, t, "-")
	v46v2 = nil
	testUnmarshalErr(&v46v2, bs46, h, t, "-")
	testDeepEqualErr(v46v1, v46v2, t, "-")

	v47v1 := map[float32]int8{10.1: 10}
	bs47, _ := testMarshalErr(v47v1, h, t, "-")
	v47v2 := make(map[float32]int8)
	testUnmarshalErr(v47v2, bs47, h, t, "-")
	testDeepEqualErr(v47v1, v47v2, t, "-")
	bs47, _ = testMarshalErr(&v47v1, h, t, "-")
	v47v2 = nil
	testUnmarshalErr(&v47v2, bs47, h, t, "-")
	testDeepEqualErr(v47v1, v47v2, t, "-")

	v48v1 := map[float32]int16{10.1: 10}
	bs48, _ := testMarshalErr(v48v1, h, t, "-")
	v48v2 := make(map[float32]int16)
	testUnmarshalErr(v48v2, bs48, h, t, "-")
	testDeepEqualErr(v48v1, v48v2, t, "-")
	bs48, _ = testMarshalErr(&v48v1, h, t, "-")
	v48v2 = nil
	testUnmarshalErr(&v48v2, bs48, h, t, "-")
	testDeepEqualErr(v48v1, v48v2, t, "-")

	v49v1 := map[float32]int32{10.1: 10}
	bs49, _ := testMarshalErr(v49v1, h, t, "-")
	v49v2 := make(map[float32]int32)
	testUnmarshalErr(v49v2, bs49, h, t, "-")
	testDeepEqualErr(v49v1, v49v2, t, "-")
	bs49, _ = testMarshalErr(&v49v1, h, t, "-")
	v49v2 = nil
	testUnmarshalErr(&v49v2, bs49, h, t, "-")
	testDeepEqualErr(v49v1, v49v2, t, "-")

	v50v1 := map[float32]int64{10.1: 10}
	bs50, _ := testMarshalErr(v50v1, h, t, "-")
	v50v2 := make(map[float32]int64)
	testUnmarshalErr(v50v2, bs50, h, t, "-")
	testDeepEqualErr(v50v1, v50v2, t, "-")
	bs50, _ = testMarshalErr(&v50v1, h, t, "-")
	v50v2 = nil
	testUnmarshalErr(&v50v2, bs50, h, t, "-")
	testDeepEqualErr(v50v1, v50v2, t, "-")

	v51v1 := map[float32]float32{10.1: 10.1}
	bs51, _ := testMarshalErr(v51v1, h, t, "-")
	v51v2 := make(map[float32]float32)
	testUnmarshalErr(v51v2, bs51, h, t, "-")
	testDeepEqualErr(v51v1, v51v2, t, "-")
	bs51, _ = testMarshalErr(&v51v1, h, t, "-")
	v51v2 = nil
	testUnmarshalErr(&v51v2, bs51, h, t, "-")
	testDeepEqualErr(v51v1, v51v2, t, "-")

	v52v1 := map[float32]float64{10.1: 10.1}
	bs52, _ := testMarshalErr(v52v1, h, t, "-")
	v52v2 := make(map[float32]float64)
	testUnmarshalErr(v52v2, bs52, h, t, "-")
	testDeepEqualErr(v52v1, v52v2, t, "-")
	bs52, _ = testMarshalErr(&v52v1, h, t, "-")
	v52v2 = nil
	testUnmarshalErr(&v52v2, bs52, h, t, "-")
	testDeepEqualErr(v52v1, v52v2, t, "-")

	v53v1 := map[float32]bool{10.1: true}
	bs53, _ := testMarshalErr(v53v1, h, t, "-")
	v53v2 := make(map[float32]bool)
	testUnmarshalErr(v53v2, bs53, h, t, "-")
	testDeepEqualErr(v53v1, v53v2, t, "-")
	bs53, _ = testMarshalErr(&v53v1, h, t, "-")
	v53v2 = nil
	testUnmarshalErr(&v53v2, bs53, h, t, "-")
	testDeepEqualErr(v53v1, v53v2, t, "-")

	v56v1 := map[float64]interface{}{10.1: "string-is-an-interface"}
	bs56, _ := testMarshalErr(v56v1, h, t, "-")
	v56v2 := make(map[float64]interface{})
	testUnmarshalErr(v56v2, bs56, h, t, "-")
	testDeepEqualErr(v56v1, v56v2, t, "-")
	bs56, _ = testMarshalErr(&v56v1, h, t, "-")
	v56v2 = nil
	testUnmarshalErr(&v56v2, bs56, h, t, "-")
	testDeepEqualErr(v56v1, v56v2, t, "-")

	v57v1 := map[float64]string{10.1: "some-string"}
	bs57, _ := testMarshalErr(v57v1, h, t, "-")
	v57v2 := make(map[float64]string)
	testUnmarshalErr(v57v2, bs57, h, t, "-")
	testDeepEqualErr(v57v1, v57v2, t, "-")
	bs57, _ = testMarshalErr(&v57v1, h, t, "-")
	v57v2 = nil
	testUnmarshalErr(&v57v2, bs57, h, t, "-")
	testDeepEqualErr(v57v1, v57v2, t, "-")

	v58v1 := map[float64]uint{10.1: 10}
	bs58, _ := testMarshalErr(v58v1, h, t, "-")
	v58v2 := make(map[float64]uint)
	testUnmarshalErr(v58v2, bs58, h, t, "-")
	testDeepEqualErr(v58v1, v58v2, t, "-")
	bs58, _ = testMarshalErr(&v58v1, h, t, "-")
	v58v2 = nil
	testUnmarshalErr(&v58v2, bs58, h, t, "-")
	testDeepEqualErr(v58v1, v58v2, t, "-")

	v59v1 := map[float64]uint8{10.1: 10}
	bs59, _ := testMarshalErr(v59v1, h, t, "-")
	v59v2 := make(map[float64]uint8)
	testUnmarshalErr(v59v2, bs59, h, t, "-")
	testDeepEqualErr(v59v1, v59v2, t, "-")
	bs59, _ = testMarshalErr(&v59v1, h, t, "-")
	v59v2 = nil
	testUnmarshalErr(&v59v2, bs59, h, t, "-")
	testDeepEqualErr(v59v1, v59v2, t, "-")

	v60v1 := map[float64]uint16{10.1: 10}
	bs60, _ := testMarshalErr(v60v1, h, t, "-")
	v60v2 := make(map[float64]uint16)
	testUnmarshalErr(v60v2, bs60, h, t, "-")
	testDeepEqualErr(v60v1, v60v2, t, "-")
	bs60, _ = testMarshalErr(&v60v1, h, t, "-")
	v60v2 = nil
	testUnmarshalErr(&v60v2, bs60, h, t, "-")
	testDeepEqualErr(v60v1, v60v2, t, "-")

	v61v1 := map[float64]uint32{10.1: 10}
	bs61, _ := testMarshalErr(v61v1, h, t, "-")
	v61v2 := make(map[float64]uint32)
	testUnmarshalErr(v61v2, bs61, h, t, "-")
	testDeepEqualErr(v61v1, v61v2, t, "-")
	bs61, _ = testMarshalErr(&v61v1, h, t, "-")
	v61v2 = nil
	testUnmarshalErr(&v61v2, bs61, h, t, "-")
	testDeepEqualErr(v61v1, v61v2, t, "-")

	v62v1 := map[float64]uint64{10.1: 10}
	bs62, _ := testMarshalErr(v62v1, h, t, "-")
	v62v2 := make(map[float64]uint64)
	testUnmarshalErr(v62v2, bs62, h, t, "-")
	testDeepEqualErr(v62v1, v62v2, t, "-")
	bs62, _ = testMarshalErr(&v62v1, h, t, "-")
	v62v2 = nil
	testUnmarshalErr(&v62v2, bs62, h, t, "-")
	testDeepEqualErr(v62v1, v62v2, t, "-")

	v63v1 := map[float64]uintptr{10.1: 10}
	bs63, _ := testMarshalErr(v63v1, h, t, "-")
	v63v2 := make(map[float64]uintptr)
	testUnmarshalErr(v63v2, bs63, h, t, "-")
	testDeepEqualErr(v63v1, v63v2, t, "-")
	bs63, _ = testMarshalErr(&v63v1, h, t, "-")
	v63v2 = nil
	testUnmarshalErr(&v63v2, bs63, h, t, "-")
	testDeepEqualErr(v63v1, v63v2, t, "-")

	v64v1 := map[float64]int{10.1: 10}
	bs64, _ := testMarshalErr(v64v1, h, t, "-")
	v64v2 := make(map[float64]int)
	testUnmarshalErr(v64v2, bs64, h, t, "-")
	testDeepEqualErr(v64v1, v64v2, t, "-")
	bs64, _ = testMarshalErr(&v64v1, h, t, "-")
	v64v2 = nil
	testUnmarshalErr(&v64v2, bs64, h, t, "-")
	testDeepEqualErr(v64v1, v64v2, t, "-")

	v65v1 := map[float64]int8{10.1: 10}
	bs65, _ := testMarshalErr(v65v1, h, t, "-")
	v65v2 := make(map[float64]int8)
	testUnmarshalErr(v65v2, bs65, h, t, "-")
	testDeepEqualErr(v65v1, v65v2, t, "-")
	bs65, _ = testMarshalErr(&v65v1, h, t, "-")
	v65v2 = nil
	testUnmarshalErr(&v65v2, bs65, h, t, "-")
	testDeepEqualErr(v65v1, v65v2, t, "-")

	v66v1 := map[float64]int16{10.1: 10}
	bs66, _ := testMarshalErr(v66v1, h, t, "-")
	v66v2 := make(map[float64]int16)
	testUnmarshalErr(v66v2, bs66, h, t, "-")
	testDeepEqualErr(v66v1, v66v2, t, "-")
	bs66, _ = testMarshalErr(&v66v1, h, t, "-")
	v66v2 = nil
	testUnmarshalErr(&v66v2, bs66, h, t, "-")
	testDeepEqualErr(v66v1, v66v2, t, "-")

	v67v1 := map[float64]int32{10.1: 10}
	bs67, _ := testMarshalErr(v67v1, h, t, "-")
	v67v2 := make(map[float64]int32)
	testUnmarshalErr(v67v2, bs67, h, t, "-")
	testDeepEqualErr(v67v1, v67v2, t, "-")
	bs67, _ = testMarshalErr(&v67v1, h, t, "-")
	v67v2 = nil
	testUnmarshalErr(&v67v2, bs67, h, t, "-")
	testDeepEqualErr(v67v1, v67v2, t, "-")

	v68v1 := map[float64]int64{10.1: 10}
	bs68, _ := testMarshalErr(v68v1, h, t, "-")
	v68v2 := make(map[float64]int64)
	testUnmarshalErr(v68v2, bs68, h, t, "-")
	testDeepEqualErr(v68v1, v68v2, t, "-")
	bs68, _ = testMarshalErr(&v68v1, h, t, "-")
	v68v2 = nil
	testUnmarshalErr(&v68v2, bs68, h, t, "-")
	testDeepEqualErr(v68v1, v68v2, t, "-")

	v69v1 := map[float64]float32{10.1: 10.1}
	bs69, _ := testMarshalErr(v69v1, h, t, "-")
	v69v2 := make(map[float64]float32)
	testUnmarshalErr(v69v2, bs69, h, t, "-")
	testDeepEqualErr(v69v1, v69v2, t, "-")
	bs69, _ = testMarshalErr(&v69v1, h, t, "-")
	v69v2 = nil
	testUnmarshalErr(&v69v2, bs69, h, t, "-")
	testDeepEqualErr(v69v1, v69v2, t, "-")

	v70v1 := map[float64]float64{10.1: 10.1}
	bs70, _ := testMarshalErr(v70v1, h, t, "-")
	v70v2 := make(map[float64]float64)
	testUnmarshalErr(v70v2, bs70, h, t, "-")
	testDeepEqualErr(v70v1, v70v2, t, "-")
	bs70, _ = testMarshalErr(&v70v1, h, t, "-")
	v70v2 = nil
	testUnmarshalErr(&v70v2, bs70, h, t, "-")
	testDeepEqualErr(v70v1, v70v2, t, "-")

	v71v1 := map[float64]bool{10.1: true}
	bs71, _ := testMarshalErr(v71v1, h, t, "-")
	v71v2 := make(map[float64]bool)
	testUnmarshalErr(v71v2, bs71, h, t, "-")
	testDeepEqualErr(v71v1, v71v2, t, "-")
	bs71, _ = testMarshalErr(&v71v1, h, t, "-")
	v71v2 = nil
	testUnmarshalErr(&v71v2, bs71, h, t, "-")
	testDeepEqualErr(v71v1, v71v2, t, "-")

	v74v1 := map[uint]interface{}{10: "string-is-an-interface"}
	bs74, _ := testMarshalErr(v74v1, h, t, "-")
	v74v2 := make(map[uint]interface{})
	testUnmarshalErr(v74v2, bs74, h, t, "-")
	testDeepEqualErr(v74v1, v74v2, t, "-")
	bs74, _ = testMarshalErr(&v74v1, h, t, "-")
	v74v2 = nil
	testUnmarshalErr(&v74v2, bs74, h, t, "-")
	testDeepEqualErr(v74v1, v74v2, t, "-")

	v75v1 := map[uint]string{10: "some-string"}
	bs75, _ := testMarshalErr(v75v1, h, t, "-")
	v75v2 := make(map[uint]string)
	testUnmarshalErr(v75v2, bs75, h, t, "-")
	testDeepEqualErr(v75v1, v75v2, t, "-")
	bs75, _ = testMarshalErr(&v75v1, h, t, "-")
	v75v2 = nil
	testUnmarshalErr(&v75v2, bs75, h, t, "-")
	testDeepEqualErr(v75v1, v75v2, t, "-")

	v76v1 := map[uint]uint{10: 10}
	bs76, _ := testMarshalErr(v76v1, h, t, "-")
	v76v2 := make(map[uint]uint)
	testUnmarshalErr(v76v2, bs76, h, t, "-")
	testDeepEqualErr(v76v1, v76v2, t, "-")
	bs76, _ = testMarshalErr(&v76v1, h, t, "-")
	v76v2 = nil
	testUnmarshalErr(&v76v2, bs76, h, t, "-")
	testDeepEqualErr(v76v1, v76v2, t, "-")

	v77v1 := map[uint]uint8{10: 10}
	bs77, _ := testMarshalErr(v77v1, h, t, "-")
	v77v2 := make(map[uint]uint8)
	testUnmarshalErr(v77v2, bs77, h, t, "-")
	testDeepEqualErr(v77v1, v77v2, t, "-")
	bs77, _ = testMarshalErr(&v77v1, h, t, "-")
	v77v2 = nil
	testUnmarshalErr(&v77v2, bs77, h, t, "-")
	testDeepEqualErr(v77v1, v77v2, t, "-")

	v78v1 := map[uint]uint16{10: 10}
	bs78, _ := testMarshalErr(v78v1, h, t, "-")
	v78v2 := make(map[uint]uint16)
	testUnmarshalErr(v78v2, bs78, h, t, "-")
	testDeepEqualErr(v78v1, v78v2, t, "-")
	bs78, _ = testMarshalErr(&v78v1, h, t, "-")
	v78v2 = nil
	testUnmarshalErr(&v78v2, bs78, h, t, "-")
	testDeepEqualErr(v78v1, v78v2, t, "-")

	v79v1 := map[uint]uint32{10: 10}
	bs79, _ := testMarshalErr(v79v1, h, t, "-")
	v79v2 := make(map[uint]uint32)
	testUnmarshalErr(v79v2, bs79, h, t, "-")
	testDeepEqualErr(v79v1, v79v2, t, "-")
	bs79, _ = testMarshalErr(&v79v1, h, t, "-")
	v79v2 = nil
	testUnmarshalErr(&v79v2, bs79, h, t, "-")
	testDeepEqualErr(v79v1, v79v2, t, "-")

	v80v1 := map[uint]uint64{10: 10}
	bs80, _ := testMarshalErr(v80v1, h, t, "-")
	v80v2 := make(map[uint]uint64)
	testUnmarshalErr(v80v2, bs80, h, t, "-")
	testDeepEqualErr(v80v1, v80v2, t, "-")
	bs80, _ = testMarshalErr(&v80v1, h, t, "-")
	v80v2 = nil
	testUnmarshalErr(&v80v2, bs80, h, t, "-")
	testDeepEqualErr(v80v1, v80v2, t, "-")

	v81v1 := map[uint]uintptr{10: 10}
	bs81, _ := testMarshalErr(v81v1, h, t, "-")
	v81v2 := make(map[uint]uintptr)
	testUnmarshalErr(v81v2, bs81, h, t, "-")
	testDeepEqualErr(v81v1, v81v2, t, "-")
	bs81, _ = testMarshalErr(&v81v1, h, t, "-")
	v81v2 = nil
	testUnmarshalErr(&v81v2, bs81, h, t, "-")
	testDeepEqualErr(v81v1, v81v2, t, "-")

	v82v1 := map[uint]int{10: 10}
	bs82, _ := testMarshalErr(v82v1, h, t, "-")
	v82v2 := make(map[uint]int)
	testUnmarshalErr(v82v2, bs82, h, t, "-")
	testDeepEqualErr(v82v1, v82v2, t, "-")
	bs82, _ = testMarshalErr(&v82v1, h, t, "-")
	v82v2 = nil
	testUnmarshalErr(&v82v2, bs82, h, t, "-")
	testDeepEqualErr(v82v1, v82v2, t, "-")

	v83v1 := map[uint]int8{10: 10}
	bs83, _ := testMarshalErr(v83v1, h, t, "-")
	v83v2 := make(map[uint]int8)
	testUnmarshalErr(v83v2, bs83, h, t, "-")
	testDeepEqualErr(v83v1, v83v2, t, "-")
	bs83, _ = testMarshalErr(&v83v1, h, t, "-")
	v83v2 = nil
	testUnmarshalErr(&v83v2, bs83, h, t, "-")
	testDeepEqualErr(v83v1, v83v2, t, "-")

	v84v1 := map[uint]int16{10: 10}
	bs84, _ := testMarshalErr(v84v1, h, t, "-")
	v84v2 := make(map[uint]int16)
	testUnmarshalErr(v84v2, bs84, h, t, "-")
	testDeepEqualErr(v84v1, v84v2, t, "-")
	bs84, _ = testMarshalErr(&v84v1, h, t, "-")
	v84v2 = nil
	testUnmarshalErr(&v84v2, bs84, h, t, "-")
	testDeepEqualErr(v84v1, v84v2, t, "-")

	v85v1 := map[uint]int32{10: 10}
	bs85, _ := testMarshalErr(v85v1, h, t, "-")
	v85v2 := make(map[uint]int32)
	testUnmarshalErr(v85v2, bs85, h, t, "-")
	testDeepEqualErr(v85v1, v85v2, t, "-")
	bs85, _ = testMarshalErr(&v85v1, h, t, "-")
	v85v2 = nil
	testUnmarshalErr(&v85v2, bs85, h, t, "-")
	testDeepEqualErr(v85v1, v85v2, t, "-")

	v86v1 := map[uint]int64{10: 10}
	bs86, _ := testMarshalErr(v86v1, h, t, "-")
	v86v2 := make(map[uint]int64)
	testUnmarshalErr(v86v2, bs86, h, t, "-")
	testDeepEqualErr(v86v1, v86v2, t, "-")
	bs86, _ = testMarshalErr(&v86v1, h, t, "-")
	v86v2 = nil
	testUnmarshalErr(&v86v2, bs86, h, t, "-")
	testDeepEqualErr(v86v1, v86v2, t, "-")

	v87v1 := map[uint]float32{10: 10.1}
	bs87, _ := testMarshalErr(v87v1, h, t, "-")
	v87v2 := make(map[uint]float32)
	testUnmarshalErr(v87v2, bs87, h, t, "-")
	testDeepEqualErr(v87v1, v87v2, t, "-")
	bs87, _ = testMarshalErr(&v87v1, h, t, "-")
	v87v2 = nil
	testUnmarshalErr(&v87v2, bs87, h, t, "-")
	testDeepEqualErr(v87v1, v87v2, t, "-")

	v88v1 := map[uint]float64{10: 10.1}
	bs88, _ := testMarshalErr(v88v1, h, t, "-")
	v88v2 := make(map[uint]float64)
	testUnmarshalErr(v88v2, bs88, h, t, "-")
	testDeepEqualErr(v88v1, v88v2, t, "-")
	bs88, _ = testMarshalErr(&v88v1, h, t, "-")
	v88v2 = nil
	testUnmarshalErr(&v88v2, bs88, h, t, "-")
	testDeepEqualErr(v88v1, v88v2, t, "-")

	v89v1 := map[uint]bool{10: true}
	bs89, _ := testMarshalErr(v89v1, h, t, "-")
	v89v2 := make(map[uint]bool)
	testUnmarshalErr(v89v2, bs89, h, t, "-")
	testDeepEqualErr(v89v1, v89v2, t, "-")
	bs89, _ = testMarshalErr(&v89v1, h, t, "-")
	v89v2 = nil
	testUnmarshalErr(&v89v2, bs89, h, t, "-")
	testDeepEqualErr(v89v1, v89v2, t, "-")

	v91v1 := map[uint8]interface{}{10: "string-is-an-interface"}
	bs91, _ := testMarshalErr(v91v1, h, t, "-")
	v91v2 := make(map[uint8]interface{})
	testUnmarshalErr(v91v2, bs91, h, t, "-")
	testDeepEqualErr(v91v1, v91v2, t, "-")
	bs91, _ = testMarshalErr(&v91v1, h, t, "-")
	v91v2 = nil
	testUnmarshalErr(&v91v2, bs91, h, t, "-")
	testDeepEqualErr(v91v1, v91v2, t, "-")

	v92v1 := map[uint8]string{10: "some-string"}
	bs92, _ := testMarshalErr(v92v1, h, t, "-")
	v92v2 := make(map[uint8]string)
	testUnmarshalErr(v92v2, bs92, h, t, "-")
	testDeepEqualErr(v92v1, v92v2, t, "-")
	bs92, _ = testMarshalErr(&v92v1, h, t, "-")
	v92v2 = nil
	testUnmarshalErr(&v92v2, bs92, h, t, "-")
	testDeepEqualErr(v92v1, v92v2, t, "-")

	v93v1 := map[uint8]uint{10: 10}
	bs93, _ := testMarshalErr(v93v1, h, t, "-")
	v93v2 := make(map[uint8]uint)
	testUnmarshalErr(v93v2, bs93, h, t, "-")
	testDeepEqualErr(v93v1, v93v2, t, "-")
	bs93, _ = testMarshalErr(&v93v1, h, t, "-")
	v93v2 = nil
	testUnmarshalErr(&v93v2, bs93, h, t, "-")
	testDeepEqualErr(v93v1, v93v2, t, "-")

	v94v1 := map[uint8]uint8{10: 10}
	bs94, _ := testMarshalErr(v94v1, h, t, "-")
	v94v2 := make(map[uint8]uint8)
	testUnmarshalErr(v94v2, bs94, h, t, "-")
	testDeepEqualErr(v94v1, v94v2, t, "-")
	bs94, _ = testMarshalErr(&v94v1, h, t, "-")
	v94v2 = nil
	testUnmarshalErr(&v94v2, bs94, h, t, "-")
	testDeepEqualErr(v94v1, v94v2, t, "-")

	v95v1 := map[uint8]uint16{10: 10}
	bs95, _ := testMarshalErr(v95v1, h, t, "-")
	v95v2 := make(map[uint8]uint16)
	testUnmarshalErr(v95v2, bs95, h, t, "-")
	testDeepEqualErr(v95v1, v95v2, t, "-")
	bs95, _ = testMarshalErr(&v95v1, h, t, "-")
	v95v2 = nil
	testUnmarshalErr(&v95v2, bs95, h, t, "-")
	testDeepEqualErr(v95v1, v95v2, t, "-")

	v96v1 := map[uint8]uint32{10: 10}
	bs96, _ := testMarshalErr(v96v1, h, t, "-")
	v96v2 := make(map[uint8]uint32)
	testUnmarshalErr(v96v2, bs96, h, t, "-")
	testDeepEqualErr(v96v1, v96v2, t, "-")
	bs96, _ = testMarshalErr(&v96v1, h, t, "-")
	v96v2 = nil
	testUnmarshalErr(&v96v2, bs96, h, t, "-")
	testDeepEqualErr(v96v1, v96v2, t, "-")

	v97v1 := map[uint8]uint64{10: 10}
	bs97, _ := testMarshalErr(v97v1, h, t, "-")
	v97v2 := make(map[uint8]uint64)
	testUnmarshalErr(v97v2, bs97, h, t, "-")
	testDeepEqualErr(v97v1, v97v2, t, "-")
	bs97, _ = testMarshalErr(&v97v1, h, t, "-")
	v97v2 = nil
	testUnmarshalErr(&v97v2, bs97, h, t, "-")
	testDeepEqualErr(v97v1, v97v2, t, "-")

	v98v1 := map[uint8]uintptr{10: 10}
	bs98, _ := testMarshalErr(v98v1, h, t, "-")
	v98v2 := make(map[uint8]uintptr)
	testUnmarshalErr(v98v2, bs98, h, t, "-")
	testDeepEqualErr(v98v1, v98v2, t, "-")
	bs98, _ = testMarshalErr(&v98v1, h, t, "-")
	v98v2 = nil
	testUnmarshalErr(&v98v2, bs98, h, t, "-")
	testDeepEqualErr(v98v1, v98v2, t, "-")

	v99v1 := map[uint8]int{10: 10}
	bs99, _ := testMarshalErr(v99v1, h, t, "-")
	v99v2 := make(map[uint8]int)
	testUnmarshalErr(v99v2, bs99, h, t, "-")
	testDeepEqualErr(v99v1, v99v2, t, "-")
	bs99, _ = testMarshalErr(&v99v1, h, t, "-")
	v99v2 = nil
	testUnmarshalErr(&v99v2, bs99, h, t, "-")
	testDeepEqualErr(v99v1, v99v2, t, "-")

	v100v1 := map[uint8]int8{10: 10}
	bs100, _ := testMarshalErr(v100v1, h, t, "-")
	v100v2 := make(map[uint8]int8)
	testUnmarshalErr(v100v2, bs100, h, t, "-")
	testDeepEqualErr(v100v1, v100v2, t, "-")
	bs100, _ = testMarshalErr(&v100v1, h, t, "-")
	v100v2 = nil
	testUnmarshalErr(&v100v2, bs100, h, t, "-")
	testDeepEqualErr(v100v1, v100v2, t, "-")

	v101v1 := map[uint8]int16{10: 10}
	bs101, _ := testMarshalErr(v101v1, h, t, "-")
	v101v2 := make(map[uint8]int16)
	testUnmarshalErr(v101v2, bs101, h, t, "-")
	testDeepEqualErr(v101v1, v101v2, t, "-")
	bs101, _ = testMarshalErr(&v101v1, h, t, "-")
	v101v2 = nil
	testUnmarshalErr(&v101v2, bs101, h, t, "-")
	testDeepEqualErr(v101v1, v101v2, t, "-")

	v102v1 := map[uint8]int32{10: 10}
	bs102, _ := testMarshalErr(v102v1, h, t, "-")
	v102v2 := make(map[uint8]int32)
	testUnmarshalErr(v102v2, bs102, h, t, "-")
	testDeepEqualErr(v102v1, v102v2, t, "-")
	bs102, _ = testMarshalErr(&v102v1, h, t, "-")
	v102v2 = nil
	testUnmarshalErr(&v102v2, bs102, h, t, "-")
	testDeepEqualErr(v102v1, v102v2, t, "-")

	v103v1 := map[uint8]int64{10: 10}
	bs103, _ := testMarshalErr(v103v1, h, t, "-")
	v103v2 := make(map[uint8]int64)
	testUnmarshalErr(v103v2, bs103, h, t, "-")
	testDeepEqualErr(v103v1, v103v2, t, "-")
	bs103, _ = testMarshalErr(&v103v1, h, t, "-")
	v103v2 = nil
	testUnmarshalErr(&v103v2, bs103, h, t, "-")
	testDeepEqualErr(v103v1, v103v2, t, "-")

	v104v1 := map[uint8]float32{10: 10.1}
	bs104, _ := testMarshalErr(v104v1, h, t, "-")
	v104v2 := make(map[uint8]float32)
	testUnmarshalErr(v104v2, bs104, h, t, "-")
	testDeepEqualErr(v104v1, v104v2, t, "-")
	bs104, _ = testMarshalErr(&v104v1, h, t, "-")
	v104v2 = nil
	testUnmarshalErr(&v104v2, bs104, h, t, "-")
	testDeepEqualErr(v104v1, v104v2, t, "-")

	v105v1 := map[uint8]float64{10: 10.1}
	bs105, _ := testMarshalErr(v105v1, h, t, "-")
	v105v2 := make(map[uint8]float64)
	testUnmarshalErr(v105v2, bs105, h, t, "-")
	testDeepEqualErr(v105v1, v105v2, t, "-")
	bs105, _ = testMarshalErr(&v105v1, h, t, "-")
	v105v2 = nil
	testUnmarshalErr(&v105v2, bs105, h, t, "-")
	testDeepEqualErr(v105v1, v105v2, t, "-")

	v106v1 := map[uint8]bool{10: true}
	bs106, _ := testMarshalErr(v106v1, h, t, "-")
	v106v2 := make(map[uint8]bool)
	testUnmarshalErr(v106v2, bs106, h, t, "-")
	testDeepEqualErr(v106v1, v106v2, t, "-")
	bs106, _ = testMarshalErr(&v106v1, h, t, "-")
	v106v2 = nil
	testUnmarshalErr(&v106v2, bs106, h, t, "-")
	testDeepEqualErr(v106v1, v106v2, t, "-")

	v109v1 := map[uint16]interface{}{10: "string-is-an-interface"}
	bs109, _ := testMarshalErr(v109v1, h, t, "-")
	v109v2 := make(map[uint16]interface{})
	testUnmarshalErr(v109v2, bs109, h, t, "-")
	testDeepEqualErr(v109v1, v109v2, t, "-")
	bs109, _ = testMarshalErr(&v109v1, h, t, "-")
	v109v2 = nil
	testUnmarshalErr(&v109v2, bs109, h, t, "-")
	testDeepEqualErr(v109v1, v109v2, t, "-")

	v110v1 := map[uint16]string{10: "some-string"}
	bs110, _ := testMarshalErr(v110v1, h, t, "-")
	v110v2 := make(map[uint16]string)
	testUnmarshalErr(v110v2, bs110, h, t, "-")
	testDeepEqualErr(v110v1, v110v2, t, "-")
	bs110, _ = testMarshalErr(&v110v1, h, t, "-")
	v110v2 = nil
	testUnmarshalErr(&v110v2, bs110, h, t, "-")
	testDeepEqualErr(v110v1, v110v2, t, "-")

	v111v1 := map[uint16]uint{10: 10}
	bs111, _ := testMarshalErr(v111v1, h, t, "-")
	v111v2 := make(map[uint16]uint)
	testUnmarshalErr(v111v2, bs111, h, t, "-")
	testDeepEqualErr(v111v1, v111v2, t, "-")
	bs111, _ = testMarshalErr(&v111v1, h, t, "-")
	v111v2 = nil
	testUnmarshalErr(&v111v2, bs111, h, t, "-")
	testDeepEqualErr(v111v1, v111v2, t, "-")

	v112v1 := map[uint16]uint8{10: 10}
	bs112, _ := testMarshalErr(v112v1, h, t, "-")
	v112v2 := make(map[uint16]uint8)
	testUnmarshalErr(v112v2, bs112, h, t, "-")
	testDeepEqualErr(v112v1, v112v2, t, "-")
	bs112, _ = testMarshalErr(&v112v1, h, t, "-")
	v112v2 = nil
	testUnmarshalErr(&v112v2, bs112, h, t, "-")
	testDeepEqualErr(v112v1, v112v2, t, "-")

	v113v1 := map[uint16]uint16{10: 10}
	bs113, _ := testMarshalErr(v113v1, h, t, "-")
	v113v2 := make(map[uint16]uint16)
	testUnmarshalErr(v113v2, bs113, h, t, "-")
	testDeepEqualErr(v113v1, v113v2, t, "-")
	bs113, _ = testMarshalErr(&v113v1, h, t, "-")
	v113v2 = nil
	testUnmarshalErr(&v113v2, bs113, h, t, "-")
	testDeepEqualErr(v113v1, v113v2, t, "-")

	v114v1 := map[uint16]uint32{10: 10}
	bs114, _ := testMarshalErr(v114v1, h, t, "-")
	v114v2 := make(map[uint16]uint32)
	testUnmarshalErr(v114v2, bs114, h, t, "-")
	testDeepEqualErr(v114v1, v114v2, t, "-")
	bs114, _ = testMarshalErr(&v114v1, h, t, "-")
	v114v2 = nil
	testUnmarshalErr(&v114v2, bs114, h, t, "-")
	testDeepEqualErr(v114v1, v114v2, t, "-")

	v115v1 := map[uint16]uint64{10: 10}
	bs115, _ := testMarshalErr(v115v1, h, t, "-")
	v115v2 := make(map[uint16]uint64)
	testUnmarshalErr(v115v2, bs115, h, t, "-")
	testDeepEqualErr(v115v1, v115v2, t, "-")
	bs115, _ = testMarshalErr(&v115v1, h, t, "-")
	v115v2 = nil
	testUnmarshalErr(&v115v2, bs115, h, t, "-")
	testDeepEqualErr(v115v1, v115v2, t, "-")

	v116v1 := map[uint16]uintptr{10: 10}
	bs116, _ := testMarshalErr(v116v1, h, t, "-")
	v116v2 := make(map[uint16]uintptr)
	testUnmarshalErr(v116v2, bs116, h, t, "-")
	testDeepEqualErr(v116v1, v116v2, t, "-")
	bs116, _ = testMarshalErr(&v116v1, h, t, "-")
	v116v2 = nil
	testUnmarshalErr(&v116v2, bs116, h, t, "-")
	testDeepEqualErr(v116v1, v116v2, t, "-")

	v117v1 := map[uint16]int{10: 10}
	bs117, _ := testMarshalErr(v117v1, h, t, "-")
	v117v2 := make(map[uint16]int)
	testUnmarshalErr(v117v2, bs117, h, t, "-")
	testDeepEqualErr(v117v1, v117v2, t, "-")
	bs117, _ = testMarshalErr(&v117v1, h, t, "-")
	v117v2 = nil
	testUnmarshalErr(&v117v2, bs117, h, t, "-")
	testDeepEqualErr(v117v1, v117v2, t, "-")

	v118v1 := map[uint16]int8{10: 10}
	bs118, _ := testMarshalErr(v118v1, h, t, "-")
	v118v2 := make(map[uint16]int8)
	testUnmarshalErr(v118v2, bs118, h, t, "-")
	testDeepEqualErr(v118v1, v118v2, t, "-")
	bs118, _ = testMarshalErr(&v118v1, h, t, "-")
	v118v2 = nil
	testUnmarshalErr(&v118v2, bs118, h, t, "-")
	testDeepEqualErr(v118v1, v118v2, t, "-")

	v119v1 := map[uint16]int16{10: 10}
	bs119, _ := testMarshalErr(v119v1, h, t, "-")
	v119v2 := make(map[uint16]int16)
	testUnmarshalErr(v119v2, bs119, h, t, "-")
	testDeepEqualErr(v119v1, v119v2, t, "-")
	bs119, _ = testMarshalErr(&v119v1, h, t, "-")
	v119v2 = nil
	testUnmarshalErr(&v119v2, bs119, h, t, "-")
	testDeepEqualErr(v119v1, v119v2, t, "-")

	v120v1 := map[uint16]int32{10: 10}
	bs120, _ := testMarshalErr(v120v1, h, t, "-")
	v120v2 := make(map[uint16]int32)
	testUnmarshalErr(v120v2, bs120, h, t, "-")
	testDeepEqualErr(v120v1, v120v2, t, "-")
	bs120, _ = testMarshalErr(&v120v1, h, t, "-")
	v120v2 = nil
	testUnmarshalErr(&v120v2, bs120, h, t, "-")
	testDeepEqualErr(v120v1, v120v2, t, "-")

	v121v1 := map[uint16]int64{10: 10}
	bs121, _ := testMarshalErr(v121v1, h, t, "-")
	v121v2 := make(map[uint16]int64)
	testUnmarshalErr(v121v2, bs121, h, t, "-")
	testDeepEqualErr(v121v1, v121v2, t, "-")
	bs121, _ = testMarshalErr(&v121v1, h, t, "-")
	v121v2 = nil
	testUnmarshalErr(&v121v2, bs121, h, t, "-")
	testDeepEqualErr(v121v1, v121v2, t, "-")

	v122v1 := map[uint16]float32{10: 10.1}
	bs122, _ := testMarshalErr(v122v1, h, t, "-")
	v122v2 := make(map[uint16]float32)
	testUnmarshalErr(v122v2, bs122, h, t, "-")
	testDeepEqualErr(v122v1, v122v2, t, "-")
	bs122, _ = testMarshalErr(&v122v1, h, t, "-")
	v122v2 = nil
	testUnmarshalErr(&v122v2, bs122, h, t, "-")
	testDeepEqualErr(v122v1, v122v2, t, "-")

	v123v1 := map[uint16]float64{10: 10.1}
	bs123, _ := testMarshalErr(v123v1, h, t, "-")
	v123v2 := make(map[uint16]float64)
	testUnmarshalErr(v123v2, bs123, h, t, "-")
	testDeepEqualErr(v123v1, v123v2, t, "-")
	bs123, _ = testMarshalErr(&v123v1, h, t, "-")
	v123v2 = nil
	testUnmarshalErr(&v123v2, bs123, h, t, "-")
	testDeepEqualErr(v123v1, v123v2, t, "-")

	v124v1 := map[uint16]bool{10: true}
	bs124, _ := testMarshalErr(v124v1, h, t, "-")
	v124v2 := make(map[uint16]bool)
	testUnmarshalErr(v124v2, bs124, h, t, "-")
	testDeepEqualErr(v124v1, v124v2, t, "-")
	bs124, _ = testMarshalErr(&v124v1, h, t, "-")
	v124v2 = nil
	testUnmarshalErr(&v124v2, bs124, h, t, "-")
	testDeepEqualErr(v124v1, v124v2, t, "-")

	v127v1 := map[uint32]interface{}{10: "string-is-an-interface"}
	bs127, _ := testMarshalErr(v127v1, h, t, "-")
	v127v2 := make(map[uint32]interface{})
	testUnmarshalErr(v127v2, bs127, h, t, "-")
	testDeepEqualErr(v127v1, v127v2, t, "-")
	bs127, _ = testMarshalErr(&v127v1, h, t, "-")
	v127v2 = nil
	testUnmarshalErr(&v127v2, bs127, h, t, "-")
	testDeepEqualErr(v127v1, v127v2, t, "-")

	v128v1 := map[uint32]string{10: "some-string"}
	bs128, _ := testMarshalErr(v128v1, h, t, "-")
	v128v2 := make(map[uint32]string)
	testUnmarshalErr(v128v2, bs128, h, t, "-")
	testDeepEqualErr(v128v1, v128v2, t, "-")
	bs128, _ = testMarshalErr(&v128v1, h, t, "-")
	v128v2 = nil
	testUnmarshalErr(&v128v2, bs128, h, t, "-")
	testDeepEqualErr(v128v1, v128v2, t, "-")

	v129v1 := map[uint32]uint{10: 10}
	bs129, _ := testMarshalErr(v129v1, h, t, "-")
	v129v2 := make(map[uint32]uint)
	testUnmarshalErr(v129v2, bs129, h, t, "-")
	testDeepEqualErr(v129v1, v129v2, t, "-")
	bs129, _ = testMarshalErr(&v129v1, h, t, "-")
	v129v2 = nil
	testUnmarshalErr(&v129v2, bs129, h, t, "-")
	testDeepEqualErr(v129v1, v129v2, t, "-")

	v130v1 := map[uint32]uint8{10: 10}
	bs130, _ := testMarshalErr(v130v1, h, t, "-")
	v130v2 := make(map[uint32]uint8)
	testUnmarshalErr(v130v2, bs130, h, t, "-")
	testDeepEqualErr(v130v1, v130v2, t, "-")
	bs130, _ = testMarshalErr(&v130v1, h, t, "-")
	v130v2 = nil
	testUnmarshalErr(&v130v2, bs130, h, t, "-")
	testDeepEqualErr(v130v1, v130v2, t, "-")

	v131v1 := map[uint32]uint16{10: 10}
	bs131, _ := testMarshalErr(v131v1, h, t, "-")
	v131v2 := make(map[uint32]uint16)
	testUnmarshalErr(v131v2, bs131, h, t, "-")
	testDeepEqualErr(v131v1, v131v2, t, "-")
	bs131, _ = testMarshalErr(&v131v1, h, t, "-")
	v131v2 = nil
	testUnmarshalErr(&v131v2, bs131, h, t, "-")
	testDeepEqualErr(v131v1, v131v2, t, "-")

	v132v1 := map[uint32]uint32{10: 10}
	bs132, _ := testMarshalErr(v132v1, h, t, "-")
	v132v2 := make(map[uint32]uint32)
	testUnmarshalErr(v132v2, bs132, h, t, "-")
	testDeepEqualErr(v132v1, v132v2, t, "-")
	bs132, _ = testMarshalErr(&v132v1, h, t, "-")
	v132v2 = nil
	testUnmarshalErr(&v132v2, bs132, h, t, "-")
	testDeepEqualErr(v132v1, v132v2, t, "-")

	v133v1 := map[uint32]uint64{10: 10}
	bs133, _ := testMarshalErr(v133v1, h, t, "-")
	v133v2 := make(map[uint32]uint64)
	testUnmarshalErr(v133v2, bs133, h, t, "-")
	testDeepEqualErr(v133v1, v133v2, t, "-")
	bs133, _ = testMarshalErr(&v133v1, h, t, "-")
	v133v2 = nil
	testUnmarshalErr(&v133v2, bs133, h, t, "-")
	testDeepEqualErr(v133v1, v133v2, t, "-")

	v134v1 := map[uint32]uintptr{10: 10}
	bs134, _ := testMarshalErr(v134v1, h, t, "-")
	v134v2 := make(map[uint32]uintptr)
	testUnmarshalErr(v134v2, bs134, h, t, "-")
	testDeepEqualErr(v134v1, v134v2, t, "-")
	bs134, _ = testMarshalErr(&v134v1, h, t, "-")
	v134v2 = nil
	testUnmarshalErr(&v134v2, bs134, h, t, "-")
	testDeepEqualErr(v134v1, v134v2, t, "-")

	v135v1 := map[uint32]int{10: 10}
	bs135, _ := testMarshalErr(v135v1, h, t, "-")
	v135v2 := make(map[uint32]int)
	testUnmarshalErr(v135v2, bs135, h, t, "-")
	testDeepEqualErr(v135v1, v135v2, t, "-")
	bs135, _ = testMarshalErr(&v135v1, h, t, "-")
	v135v2 = nil
	testUnmarshalErr(&v135v2, bs135, h, t, "-")
	testDeepEqualErr(v135v1, v135v2, t, "-")

	v136v1 := map[uint32]int8{10: 10}
	bs136, _ := testMarshalErr(v136v1, h, t, "-")
	v136v2 := make(map[uint32]int8)
	testUnmarshalErr(v136v2, bs136, h, t, "-")
	testDeepEqualErr(v136v1, v136v2, t, "-")
	bs136, _ = testMarshalErr(&v136v1, h, t, "-")
	v136v2 = nil
	testUnmarshalErr(&v136v2, bs136, h, t, "-")
	testDeepEqualErr(v136v1, v136v2, t, "-")

	v137v1 := map[uint32]int16{10: 10}
	bs137, _ := testMarshalErr(v137v1, h, t, "-")
	v137v2 := make(map[uint32]int16)
	testUnmarshalErr(v137v2, bs137, h, t, "-")
	testDeepEqualErr(v137v1, v137v2, t, "-")
	bs137, _ = testMarshalErr(&v137v1, h, t, "-")
	v137v2 = nil
	testUnmarshalErr(&v137v2, bs137, h, t, "-")
	testDeepEqualErr(v137v1, v137v2, t, "-")

	v138v1 := map[uint32]int32{10: 10}
	bs138, _ := testMarshalErr(v138v1, h, t, "-")
	v138v2 := make(map[uint32]int32)
	testUnmarshalErr(v138v2, bs138, h, t, "-")
	testDeepEqualErr(v138v1, v138v2, t, "-")
	bs138, _ = testMarshalErr(&v138v1, h, t, "-")
	v138v2 = nil
	testUnmarshalErr(&v138v2, bs138, h, t, "-")
	testDeepEqualErr(v138v1, v138v2, t, "-")

	v139v1 := map[uint32]int64{10: 10}
	bs139, _ := testMarshalErr(v139v1, h, t, "-")
	v139v2 := make(map[uint32]int64)
	testUnmarshalErr(v139v2, bs139, h, t, "-")
	testDeepEqualErr(v139v1, v139v2, t, "-")
	bs139, _ = testMarshalErr(&v139v1, h, t, "-")
	v139v2 = nil
	testUnmarshalErr(&v139v2, bs139, h, t, "-")
	testDeepEqualErr(v139v1, v139v2, t, "-")

	v140v1 := map[uint32]float32{10: 10.1}
	bs140, _ := testMarshalErr(v140v1, h, t, "-")
	v140v2 := make(map[uint32]float32)
	testUnmarshalErr(v140v2, bs140, h, t, "-")
	testDeepEqualErr(v140v1, v140v2, t, "-")
	bs140, _ = testMarshalErr(&v140v1, h, t, "-")
	v140v2 = nil
	testUnmarshalErr(&v140v2, bs140, h, t, "-")
	testDeepEqualErr(v140v1, v140v2, t, "-")

	v141v1 := map[uint32]float64{10: 10.1}
	bs141, _ := testMarshalErr(v141v1, h, t, "-")
	v141v2 := make(map[uint32]float64)
	testUnmarshalErr(v141v2, bs141, h, t, "-")
	testDeepEqualErr(v141v1, v141v2, t, "-")
	bs141, _ = testMarshalErr(&v141v1, h, t, "-")
	v141v2 = nil
	testUnmarshalErr(&v141v2, bs141, h, t, "-")
	testDeepEqualErr(v141v1, v141v2, t, "-")

	v142v1 := map[uint32]bool{10: true}
	bs142, _ := testMarshalErr(v142v1, h, t, "-")
	v142v2 := make(map[uint32]bool)
	testUnmarshalErr(v142v2, bs142, h, t, "-")
	testDeepEqualErr(v142v1, v142v2, t, "-")
	bs142, _ = testMarshalErr(&v142v1, h, t, "-")
	v142v2 = nil
	testUnmarshalErr(&v142v2, bs142, h, t, "-")
	testDeepEqualErr(v142v1, v142v2, t, "-")

	v145v1 := map[uint64]interface{}{10: "string-is-an-interface"}
	bs145, _ := testMarshalErr(v145v1, h, t, "-")
	v145v2 := make(map[uint64]interface{})
	testUnmarshalErr(v145v2, bs145, h, t, "-")
	testDeepEqualErr(v145v1, v145v2, t, "-")
	bs145, _ = testMarshalErr(&v145v1, h, t, "-")
	v145v2 = nil
	testUnmarshalErr(&v145v2, bs145, h, t, "-")
	testDeepEqualErr(v145v1, v145v2, t, "-")

	v146v1 := map[uint64]string{10: "some-string"}
	bs146, _ := testMarshalErr(v146v1, h, t, "-")
	v146v2 := make(map[uint64]string)
	testUnmarshalErr(v146v2, bs146, h, t, "-")
	testDeepEqualErr(v146v1, v146v2, t, "-")
	bs146, _ = testMarshalErr(&v146v1, h, t, "-")
	v146v2 = nil
	testUnmarshalErr(&v146v2, bs146, h, t, "-")
	testDeepEqualErr(v146v1, v146v2, t, "-")

	v147v1 := map[uint64]uint{10: 10}
	bs147, _ := testMarshalErr(v147v1, h, t, "-")
	v147v2 := make(map[uint64]uint)
	testUnmarshalErr(v147v2, bs147, h, t, "-")
	testDeepEqualErr(v147v1, v147v2, t, "-")
	bs147, _ = testMarshalErr(&v147v1, h, t, "-")
	v147v2 = nil
	testUnmarshalErr(&v147v2, bs147, h, t, "-")
	testDeepEqualErr(v147v1, v147v2, t, "-")

	v148v1 := map[uint64]uint8{10: 10}
	bs148, _ := testMarshalErr(v148v1, h, t, "-")
	v148v2 := make(map[uint64]uint8)
	testUnmarshalErr(v148v2, bs148, h, t, "-")
	testDeepEqualErr(v148v1, v148v2, t, "-")
	bs148, _ = testMarshalErr(&v148v1, h, t, "-")
	v148v2 = nil
	testUnmarshalErr(&v148v2, bs148, h, t, "-")
	testDeepEqualErr(v148v1, v148v2, t, "-")

	v149v1 := map[uint64]uint16{10: 10}
	bs149, _ := testMarshalErr(v149v1, h, t, "-")
	v149v2 := make(map[uint64]uint16)
	testUnmarshalErr(v149v2, bs149, h, t, "-")
	testDeepEqualErr(v149v1, v149v2, t, "-")
	bs149, _ = testMarshalErr(&v149v1, h, t, "-")
	v149v2 = nil
	testUnmarshalErr(&v149v2, bs149, h, t, "-")
	testDeepEqualErr(v149v1, v149v2, t, "-")

	v150v1 := map[uint64]uint32{10: 10}
	bs150, _ := testMarshalErr(v150v1, h, t, "-")
	v150v2 := make(map[uint64]uint32)
	testUnmarshalErr(v150v2, bs150, h, t, "-")
	testDeepEqualErr(v150v1, v150v2, t, "-")
	bs150, _ = testMarshalErr(&v150v1, h, t, "-")
	v150v2 = nil
	testUnmarshalErr(&v150v2, bs150, h, t, "-")
	testDeepEqualErr(v150v1, v150v2, t, "-")

	v151v1 := map[uint64]uint64{10: 10}
	bs151, _ := testMarshalErr(v151v1, h, t, "-")
	v151v2 := make(map[uint64]uint64)
	testUnmarshalErr(v151v2, bs151, h, t, "-")
	testDeepEqualErr(v151v1, v151v2, t, "-")
	bs151, _ = testMarshalErr(&v151v1, h, t, "-")
	v151v2 = nil
	testUnmarshalErr(&v151v2, bs151, h, t, "-")
	testDeepEqualErr(v151v1, v151v2, t, "-")

	v152v1 := map[uint64]uintptr{10: 10}
	bs152, _ := testMarshalErr(v152v1, h, t, "-")
	v152v2 := make(map[uint64]uintptr)
	testUnmarshalErr(v152v2, bs152, h, t, "-")
	testDeepEqualErr(v152v1, v152v2, t, "-")
	bs152, _ = testMarshalErr(&v152v1, h, t, "-")
	v152v2 = nil
	testUnmarshalErr(&v152v2, bs152, h, t, "-")
	testDeepEqualErr(v152v1, v152v2, t, "-")

	v153v1 := map[uint64]int{10: 10}
	bs153, _ := testMarshalErr(v153v1, h, t, "-")
	v153v2 := make(map[uint64]int)
	testUnmarshalErr(v153v2, bs153, h, t, "-")
	testDeepEqualErr(v153v1, v153v2, t, "-")
	bs153, _ = testMarshalErr(&v153v1, h, t, "-")
	v153v2 = nil
	testUnmarshalErr(&v153v2, bs153, h, t, "-")
	testDeepEqualErr(v153v1, v153v2, t, "-")

	v154v1 := map[uint64]int8{10: 10}
	bs154, _ := testMarshalErr(v154v1, h, t, "-")
	v154v2 := make(map[uint64]int8)
	testUnmarshalErr(v154v2, bs154, h, t, "-")
	testDeepEqualErr(v154v1, v154v2, t, "-")
	bs154, _ = testMarshalErr(&v154v1, h, t, "-")
	v154v2 = nil
	testUnmarshalErr(&v154v2, bs154, h, t, "-")
	testDeepEqualErr(v154v1, v154v2, t, "-")

	v155v1 := map[uint64]int16{10: 10}
	bs155, _ := testMarshalErr(v155v1, h, t, "-")
	v155v2 := make(map[uint64]int16)
	testUnmarshalErr(v155v2, bs155, h, t, "-")
	testDeepEqualErr(v155v1, v155v2, t, "-")
	bs155, _ = testMarshalErr(&v155v1, h, t, "-")
	v155v2 = nil
	testUnmarshalErr(&v155v2, bs155, h, t, "-")
	testDeepEqualErr(v155v1, v155v2, t, "-")

	v156v1 := map[uint64]int32{10: 10}
	bs156, _ := testMarshalErr(v156v1, h, t, "-")
	v156v2 := make(map[uint64]int32)
	testUnmarshalErr(v156v2, bs156, h, t, "-")
	testDeepEqualErr(v156v1, v156v2, t, "-")
	bs156, _ = testMarshalErr(&v156v1, h, t, "-")
	v156v2 = nil
	testUnmarshalErr(&v156v2, bs156, h, t, "-")
	testDeepEqualErr(v156v1, v156v2, t, "-")

	v157v1 := map[uint64]int64{10: 10}
	bs157, _ := testMarshalErr(v157v1, h, t, "-")
	v157v2 := make(map[uint64]int64)
	testUnmarshalErr(v157v2, bs157, h, t, "-")
	testDeepEqualErr(v157v1, v157v2, t, "-")
	bs157, _ = testMarshalErr(&v157v1, h, t, "-")
	v157v2 = nil
	testUnmarshalErr(&v157v2, bs157, h, t, "-")
	testDeepEqualErr(v157v1, v157v2, t, "-")

	v158v1 := map[uint64]float32{10: 10.1}
	bs158, _ := testMarshalErr(v158v1, h, t, "-")
	v158v2 := make(map[uint64]float32)
	testUnmarshalErr(v158v2, bs158, h, t, "-")
	testDeepEqualErr(v158v1, v158v2, t, "-")
	bs158, _ = testMarshalErr(&v158v1, h, t, "-")
	v158v2 = nil
	testUnmarshalErr(&v158v2, bs158, h, t, "-")
	testDeepEqualErr(v158v1, v158v2, t, "-")

	v159v1 := map[uint64]float64{10: 10.1}
	bs159, _ := testMarshalErr(v159v1, h, t, "-")
	v159v2 := make(map[uint64]float64)
	testUnmarshalErr(v159v2, bs159, h, t, "-")
	testDeepEqualErr(v159v1, v159v2, t, "-")
	bs159, _ = testMarshalErr(&v159v1, h, t, "-")
	v159v2 = nil
	testUnmarshalErr(&v159v2, bs159, h, t, "-")
	testDeepEqualErr(v159v1, v159v2, t, "-")

	v160v1 := map[uint64]bool{10: true}
	bs160, _ := testMarshalErr(v160v1, h, t, "-")
	v160v2 := make(map[uint64]bool)
	testUnmarshalErr(v160v2, bs160, h, t, "-")
	testDeepEqualErr(v160v1, v160v2, t, "-")
	bs160, _ = testMarshalErr(&v160v1, h, t, "-")
	v160v2 = nil
	testUnmarshalErr(&v160v2, bs160, h, t, "-")
	testDeepEqualErr(v160v1, v160v2, t, "-")

	v163v1 := map[uintptr]interface{}{10: "string-is-an-interface"}
	bs163, _ := testMarshalErr(v163v1, h, t, "-")
	v163v2 := make(map[uintptr]interface{})
	testUnmarshalErr(v163v2, bs163, h, t, "-")
	testDeepEqualErr(v163v1, v163v2, t, "-")
	bs163, _ = testMarshalErr(&v163v1, h, t, "-")
	v163v2 = nil
	testUnmarshalErr(&v163v2, bs163, h, t, "-")
	testDeepEqualErr(v163v1, v163v2, t, "-")

	v164v1 := map[uintptr]string{10: "some-string"}
	bs164, _ := testMarshalErr(v164v1, h, t, "-")
	v164v2 := make(map[uintptr]string)
	testUnmarshalErr(v164v2, bs164, h, t, "-")
	testDeepEqualErr(v164v1, v164v2, t, "-")
	bs164, _ = testMarshalErr(&v164v1, h, t, "-")
	v164v2 = nil
	testUnmarshalErr(&v164v2, bs164, h, t, "-")
	testDeepEqualErr(v164v1, v164v2, t, "-")

	v165v1 := map[uintptr]uint{10: 10}
	bs165, _ := testMarshalErr(v165v1, h, t, "-")
	v165v2 := make(map[uintptr]uint)
	testUnmarshalErr(v165v2, bs165, h, t, "-")
	testDeepEqualErr(v165v1, v165v2, t, "-")
	bs165, _ = testMarshalErr(&v165v1, h, t, "-")
	v165v2 = nil
	testUnmarshalErr(&v165v2, bs165, h, t, "-")
	testDeepEqualErr(v165v1, v165v2, t, "-")

	v166v1 := map[uintptr]uint8{10: 10}
	bs166, _ := testMarshalErr(v166v1, h, t, "-")
	v166v2 := make(map[uintptr]uint8)
	testUnmarshalErr(v166v2, bs166, h, t, "-")
	testDeepEqualErr(v166v1, v166v2, t, "-")
	bs166, _ = testMarshalErr(&v166v1, h, t, "-")
	v166v2 = nil
	testUnmarshalErr(&v166v2, bs166, h, t, "-")
	testDeepEqualErr(v166v1, v166v2, t, "-")

	v167v1 := map[uintptr]uint16{10: 10}
	bs167, _ := testMarshalErr(v167v1, h, t, "-")
	v167v2 := make(map[uintptr]uint16)
	testUnmarshalErr(v167v2, bs167, h, t, "-")
	testDeepEqualErr(v167v1, v167v2, t, "-")
	bs167, _ = testMarshalErr(&v167v1, h, t, "-")
	v167v2 = nil
	testUnmarshalErr(&v167v2, bs167, h, t, "-")
	testDeepEqualErr(v167v1, v167v2, t, "-")

	v168v1 := map[uintptr]uint32{10: 10}
	bs168, _ := testMarshalErr(v168v1, h, t, "-")
	v168v2 := make(map[uintptr]uint32)
	testUnmarshalErr(v168v2, bs168, h, t, "-")
	testDeepEqualErr(v168v1, v168v2, t, "-")
	bs168, _ = testMarshalErr(&v168v1, h, t, "-")
	v168v2 = nil
	testUnmarshalErr(&v168v2, bs168, h, t, "-")
	testDeepEqualErr(v168v1, v168v2, t, "-")

	v169v1 := map[uintptr]uint64{10: 10}
	bs169, _ := testMarshalErr(v169v1, h, t, "-")
	v169v2 := make(map[uintptr]uint64)
	testUnmarshalErr(v169v2, bs169, h, t, "-")
	testDeepEqualErr(v169v1, v169v2, t, "-")
	bs169, _ = testMarshalErr(&v169v1, h, t, "-")
	v169v2 = nil
	testUnmarshalErr(&v169v2, bs169, h, t, "-")
	testDeepEqualErr(v169v1, v169v2, t, "-")

	v170v1 := map[uintptr]uintptr{10: 10}
	bs170, _ := testMarshalErr(v170v1, h, t, "-")
	v170v2 := make(map[uintptr]uintptr)
	testUnmarshalErr(v170v2, bs170, h, t, "-")
	testDeepEqualErr(v170v1, v170v2, t, "-")
	bs170, _ = testMarshalErr(&v170v1, h, t, "-")
	v170v2 = nil
	testUnmarshalErr(&v170v2, bs170, h, t, "-")
	testDeepEqualErr(v170v1, v170v2, t, "-")

	v171v1 := map[uintptr]int{10: 10}
	bs171, _ := testMarshalErr(v171v1, h, t, "-")
	v171v2 := make(map[uintptr]int)
	testUnmarshalErr(v171v2, bs171, h, t, "-")
	testDeepEqualErr(v171v1, v171v2, t, "-")
	bs171, _ = testMarshalErr(&v171v1, h, t, "-")
	v171v2 = nil
	testUnmarshalErr(&v171v2, bs171, h, t, "-")
	testDeepEqualErr(v171v1, v171v2, t, "-")

	v172v1 := map[uintptr]int8{10: 10}
	bs172, _ := testMarshalErr(v172v1, h, t, "-")
	v172v2 := make(map[uintptr]int8)
	testUnmarshalErr(v172v2, bs172, h, t, "-")
	testDeepEqualErr(v172v1, v172v2, t, "-")
	bs172, _ = testMarshalErr(&v172v1, h, t, "-")
	v172v2 = nil
	testUnmarshalErr(&v172v2, bs172, h, t, "-")
	testDeepEqualErr(v172v1, v172v2, t, "-")

	v173v1 := map[uintptr]int16{10: 10}
	bs173, _ := testMarshalErr(v173v1, h, t, "-")
	v173v2 := make(map[uintptr]int16)
	testUnmarshalErr(v173v2, bs173, h, t, "-")
	testDeepEqualErr(v173v1, v173v2, t, "-")
	bs173, _ = testMarshalErr(&v173v1, h, t, "-")
	v173v2 = nil
	testUnmarshalErr(&v173v2, bs173, h, t, "-")
	testDeepEqualErr(v173v1, v173v2, t, "-")

	v174v1 := map[uintptr]int32{10: 10}
	bs174, _ := testMarshalErr(v174v1, h, t, "-")
	v174v2 := make(map[uintptr]int32)
	testUnmarshalErr(v174v2, bs174, h, t, "-")
	testDeepEqualErr(v174v1, v174v2, t, "-")
	bs174, _ = testMarshalErr(&v174v1, h, t, "-")
	v174v2 = nil
	testUnmarshalErr(&v174v2, bs174, h, t, "-")
	testDeepEqualErr(v174v1, v174v2, t, "-")

	v175v1 := map[uintptr]int64{10: 10}
	bs175, _ := testMarshalErr(v175v1, h, t, "-")
	v175v2 := make(map[uintptr]int64)
	testUnmarshalErr(v175v2, bs175, h, t, "-")
	testDeepEqualErr(v175v1, v175v2, t, "-")
	bs175, _ = testMarshalErr(&v175v1, h, t, "-")
	v175v2 = nil
	testUnmarshalErr(&v175v2, bs175, h, t, "-")
	testDeepEqualErr(v175v1, v175v2, t, "-")

	v176v1 := map[uintptr]float32{10: 10.1}
	bs176, _ := testMarshalErr(v176v1, h, t, "-")
	v176v2 := make(map[uintptr]float32)
	testUnmarshalErr(v176v2, bs176, h, t, "-")
	testDeepEqualErr(v176v1, v176v2, t, "-")
	bs176, _ = testMarshalErr(&v176v1, h, t, "-")
	v176v2 = nil
	testUnmarshalErr(&v176v2, bs176, h, t, "-")
	testDeepEqualErr(v176v1, v176v2, t, "-")

	v177v1 := map[uintptr]float64{10: 10.1}
	bs177, _ := testMarshalErr(v177v1, h, t, "-")
	v177v2 := make(map[uintptr]float64)
	testUnmarshalErr(v177v2, bs177, h, t, "-")
	testDeepEqualErr(v177v1, v177v2, t, "-")
	bs177, _ = testMarshalErr(&v177v1, h, t, "-")
	v177v2 = nil
	testUnmarshalErr(&v177v2, bs177, h, t, "-")
	testDeepEqualErr(v177v1, v177v2, t, "-")

	v178v1 := map[uintptr]bool{10: true}
	bs178, _ := testMarshalErr(v178v1, h, t, "-")
	v178v2 := make(map[uintptr]bool)
	testUnmarshalErr(v178v2, bs178, h, t, "-")
	testDeepEqualErr(v178v1, v178v2, t, "-")
	bs178, _ = testMarshalErr(&v178v1, h, t, "-")
	v178v2 = nil
	testUnmarshalErr(&v178v2, bs178, h, t, "-")
	testDeepEqualErr(v178v1, v178v2, t, "-")

	v181v1 := map[int]interface{}{10: "string-is-an-interface"}
	bs181, _ := testMarshalErr(v181v1, h, t, "-")
	v181v2 := make(map[int]interface{})
	testUnmarshalErr(v181v2, bs181, h, t, "-")
	testDeepEqualErr(v181v1, v181v2, t, "-")
	bs181, _ = testMarshalErr(&v181v1, h, t, "-")
	v181v2 = nil
	testUnmarshalErr(&v181v2, bs181, h, t, "-")
	testDeepEqualErr(v181v1, v181v2, t, "-")

	v182v1 := map[int]string{10: "some-string"}
	bs182, _ := testMarshalErr(v182v1, h, t, "-")
	v182v2 := make(map[int]string)
	testUnmarshalErr(v182v2, bs182, h, t, "-")
	testDeepEqualErr(v182v1, v182v2, t, "-")
	bs182, _ = testMarshalErr(&v182v1, h, t, "-")
	v182v2 = nil
	testUnmarshalErr(&v182v2, bs182, h, t, "-")
	testDeepEqualErr(v182v1, v182v2, t, "-")

	v183v1 := map[int]uint{10: 10}
	bs183, _ := testMarshalErr(v183v1, h, t, "-")
	v183v2 := make(map[int]uint)
	testUnmarshalErr(v183v2, bs183, h, t, "-")
	testDeepEqualErr(v183v1, v183v2, t, "-")
	bs183, _ = testMarshalErr(&v183v1, h, t, "-")
	v183v2 = nil
	testUnmarshalErr(&v183v2, bs183, h, t, "-")
	testDeepEqualErr(v183v1, v183v2, t, "-")

	v184v1 := map[int]uint8{10: 10}
	bs184, _ := testMarshalErr(v184v1, h, t, "-")
	v184v2 := make(map[int]uint8)
	testUnmarshalErr(v184v2, bs184, h, t, "-")
	testDeepEqualErr(v184v1, v184v2, t, "-")
	bs184, _ = testMarshalErr(&v184v1, h, t, "-")
	v184v2 = nil
	testUnmarshalErr(&v184v2, bs184, h, t, "-")
	testDeepEqualErr(v184v1, v184v2, t, "-")

	v185v1 := map[int]uint16{10: 10}
	bs185, _ := testMarshalErr(v185v1, h, t, "-")
	v185v2 := make(map[int]uint16)
	testUnmarshalErr(v185v2, bs185, h, t, "-")
	testDeepEqualErr(v185v1, v185v2, t, "-")
	bs185, _ = testMarshalErr(&v185v1, h, t, "-")
	v185v2 = nil
	testUnmarshalErr(&v185v2, bs185, h, t, "-")
	testDeepEqualErr(v185v1, v185v2, t, "-")

	v186v1 := map[int]uint32{10: 10}
	bs186, _ := testMarshalErr(v186v1, h, t, "-")
	v186v2 := make(map[int]uint32)
	testUnmarshalErr(v186v2, bs186, h, t, "-")
	testDeepEqualErr(v186v1, v186v2, t, "-")
	bs186, _ = testMarshalErr(&v186v1, h, t, "-")
	v186v2 = nil
	testUnmarshalErr(&v186v2, bs186, h, t, "-")
	testDeepEqualErr(v186v1, v186v2, t, "-")

	v187v1 := map[int]uint64{10: 10}
	bs187, _ := testMarshalErr(v187v1, h, t, "-")
	v187v2 := make(map[int]uint64)
	testUnmarshalErr(v187v2, bs187, h, t, "-")
	testDeepEqualErr(v187v1, v187v2, t, "-")
	bs187, _ = testMarshalErr(&v187v1, h, t, "-")
	v187v2 = nil
	testUnmarshalErr(&v187v2, bs187, h, t, "-")
	testDeepEqualErr(v187v1, v187v2, t, "-")

	v188v1 := map[int]uintptr{10: 10}
	bs188, _ := testMarshalErr(v188v1, h, t, "-")
	v188v2 := make(map[int]uintptr)
	testUnmarshalErr(v188v2, bs188, h, t, "-")
	testDeepEqualErr(v188v1, v188v2, t, "-")
	bs188, _ = testMarshalErr(&v188v1, h, t, "-")
	v188v2 = nil
	testUnmarshalErr(&v188v2, bs188, h, t, "-")
	testDeepEqualErr(v188v1, v188v2, t, "-")

	v189v1 := map[int]int{10: 10}
	bs189, _ := testMarshalErr(v189v1, h, t, "-")
	v189v2 := make(map[int]int)
	testUnmarshalErr(v189v2, bs189, h, t, "-")
	testDeepEqualErr(v189v1, v189v2, t, "-")
	bs189, _ = testMarshalErr(&v189v1, h, t, "-")
	v189v2 = nil
	testUnmarshalErr(&v189v2, bs189, h, t, "-")
	testDeepEqualErr(v189v1, v189v2, t, "-")

	v190v1 := map[int]int8{10: 10}
	bs190, _ := testMarshalErr(v190v1, h, t, "-")
	v190v2 := make(map[int]int8)
	testUnmarshalErr(v190v2, bs190, h, t, "-")
	testDeepEqualErr(v190v1, v190v2, t, "-")
	bs190, _ = testMarshalErr(&v190v1, h, t, "-")
	v190v2 = nil
	testUnmarshalErr(&v190v2, bs190, h, t, "-")
	testDeepEqualErr(v190v1, v190v2, t, "-")

	v191v1 := map[int]int16{10: 10}
	bs191, _ := testMarshalErr(v191v1, h, t, "-")
	v191v2 := make(map[int]int16)
	testUnmarshalErr(v191v2, bs191, h, t, "-")
	testDeepEqualErr(v191v1, v191v2, t, "-")
	bs191, _ = testMarshalErr(&v191v1, h, t, "-")
	v191v2 = nil
	testUnmarshalErr(&v191v2, bs191, h, t, "-")
	testDeepEqualErr(v191v1, v191v2, t, "-")

	v192v1 := map[int]int32{10: 10}
	bs192, _ := testMarshalErr(v192v1, h, t, "-")
	v192v2 := make(map[int]int32)
	testUnmarshalErr(v192v2, bs192, h, t, "-")
	testDeepEqualErr(v192v1, v192v2, t, "-")
	bs192, _ = testMarshalErr(&v192v1, h, t, "-")
	v192v2 = nil
	testUnmarshalErr(&v192v2, bs192, h, t, "-")
	testDeepEqualErr(v192v1, v192v2, t, "-")

	v193v1 := map[int]int64{10: 10}
	bs193, _ := testMarshalErr(v193v1, h, t, "-")
	v193v2 := make(map[int]int64)
	testUnmarshalErr(v193v2, bs193, h, t, "-")
	testDeepEqualErr(v193v1, v193v2, t, "-")
	bs193, _ = testMarshalErr(&v193v1, h, t, "-")
	v193v2 = nil
	testUnmarshalErr(&v193v2, bs193, h, t, "-")
	testDeepEqualErr(v193v1, v193v2, t, "-")

	v194v1 := map[int]float32{10: 10.1}
	bs194, _ := testMarshalErr(v194v1, h, t, "-")
	v194v2 := make(map[int]float32)
	testUnmarshalErr(v194v2, bs194, h, t, "-")
	testDeepEqualErr(v194v1, v194v2, t, "-")
	bs194, _ = testMarshalErr(&v194v1, h, t, "-")
	v194v2 = nil
	testUnmarshalErr(&v194v2, bs194, h, t, "-")
	testDeepEqualErr(v194v1, v194v2, t, "-")

	v195v1 := map[int]float64{10: 10.1}
	bs195, _ := testMarshalErr(v195v1, h, t, "-")
	v195v2 := make(map[int]float64)
	testUnmarshalErr(v195v2, bs195, h, t, "-")
	testDeepEqualErr(v195v1, v195v2, t, "-")
	bs195, _ = testMarshalErr(&v195v1, h, t, "-")
	v195v2 = nil
	testUnmarshalErr(&v195v2, bs195, h, t, "-")
	testDeepEqualErr(v195v1, v195v2, t, "-")

	v196v1 := map[int]bool{10: true}
	bs196, _ := testMarshalErr(v196v1, h, t, "-")
	v196v2 := make(map[int]bool)
	testUnmarshalErr(v196v2, bs196, h, t, "-")
	testDeepEqualErr(v196v1, v196v2, t, "-")
	bs196, _ = testMarshalErr(&v196v1, h, t, "-")
	v196v2 = nil
	testUnmarshalErr(&v196v2, bs196, h, t, "-")
	testDeepEqualErr(v196v1, v196v2, t, "-")

	v199v1 := map[int8]interface{}{10: "string-is-an-interface"}
	bs199, _ := testMarshalErr(v199v1, h, t, "-")
	v199v2 := make(map[int8]interface{})
	testUnmarshalErr(v199v2, bs199, h, t, "-")
	testDeepEqualErr(v199v1, v199v2, t, "-")
	bs199, _ = testMarshalErr(&v199v1, h, t, "-")
	v199v2 = nil
	testUnmarshalErr(&v199v2, bs199, h, t, "-")
	testDeepEqualErr(v199v1, v199v2, t, "-")

	v200v1 := map[int8]string{10: "some-string"}
	bs200, _ := testMarshalErr(v200v1, h, t, "-")
	v200v2 := make(map[int8]string)
	testUnmarshalErr(v200v2, bs200, h, t, "-")
	testDeepEqualErr(v200v1, v200v2, t, "-")
	bs200, _ = testMarshalErr(&v200v1, h, t, "-")
	v200v2 = nil
	testUnmarshalErr(&v200v2, bs200, h, t, "-")
	testDeepEqualErr(v200v1, v200v2, t, "-")

	v201v1 := map[int8]uint{10: 10}
	bs201, _ := testMarshalErr(v201v1, h, t, "-")
	v201v2 := make(map[int8]uint)
	testUnmarshalErr(v201v2, bs201, h, t, "-")
	testDeepEqualErr(v201v1, v201v2, t, "-")
	bs201, _ = testMarshalErr(&v201v1, h, t, "-")
	v201v2 = nil
	testUnmarshalErr(&v201v2, bs201, h, t, "-")
	testDeepEqualErr(v201v1, v201v2, t, "-")

	v202v1 := map[int8]uint8{10: 10}
	bs202, _ := testMarshalErr(v202v1, h, t, "-")
	v202v2 := make(map[int8]uint8)
	testUnmarshalErr(v202v2, bs202, h, t, "-")
	testDeepEqualErr(v202v1, v202v2, t, "-")
	bs202, _ = testMarshalErr(&v202v1, h, t, "-")
	v202v2 = nil
	testUnmarshalErr(&v202v2, bs202, h, t, "-")
	testDeepEqualErr(v202v1, v202v2, t, "-")

	v203v1 := map[int8]uint16{10: 10}
	bs203, _ := testMarshalErr(v203v1, h, t, "-")
	v203v2 := make(map[int8]uint16)
	testUnmarshalErr(v203v2, bs203, h, t, "-")
	testDeepEqualErr(v203v1, v203v2, t, "-")
	bs203, _ = testMarshalErr(&v203v1, h, t, "-")
	v203v2 = nil
	testUnmarshalErr(&v203v2, bs203, h, t, "-")
	testDeepEqualErr(v203v1, v203v2, t, "-")

	v204v1 := map[int8]uint32{10: 10}
	bs204, _ := testMarshalErr(v204v1, h, t, "-")
	v204v2 := make(map[int8]uint32)
	testUnmarshalErr(v204v2, bs204, h, t, "-")
	testDeepEqualErr(v204v1, v204v2, t, "-")
	bs204, _ = testMarshalErr(&v204v1, h, t, "-")
	v204v2 = nil
	testUnmarshalErr(&v204v2, bs204, h, t, "-")
	testDeepEqualErr(v204v1, v204v2, t, "-")

	v205v1 := map[int8]uint64{10: 10}
	bs205, _ := testMarshalErr(v205v1, h, t, "-")
	v205v2 := make(map[int8]uint64)
	testUnmarshalErr(v205v2, bs205, h, t, "-")
	testDeepEqualErr(v205v1, v205v2, t, "-")
	bs205, _ = testMarshalErr(&v205v1, h, t, "-")
	v205v2 = nil
	testUnmarshalErr(&v205v2, bs205, h, t, "-")
	testDeepEqualErr(v205v1, v205v2, t, "-")

	v206v1 := map[int8]uintptr{10: 10}
	bs206, _ := testMarshalErr(v206v1, h, t, "-")
	v206v2 := make(map[int8]uintptr)
	testUnmarshalErr(v206v2, bs206, h, t, "-")
	testDeepEqualErr(v206v1, v206v2, t, "-")
	bs206, _ = testMarshalErr(&v206v1, h, t, "-")
	v206v2 = nil
	testUnmarshalErr(&v206v2, bs206, h, t, "-")
	testDeepEqualErr(v206v1, v206v2, t, "-")

	v207v1 := map[int8]int{10: 10}
	bs207, _ := testMarshalErr(v207v1, h, t, "-")
	v207v2 := make(map[int8]int)
	testUnmarshalErr(v207v2, bs207, h, t, "-")
	testDeepEqualErr(v207v1, v207v2, t, "-")
	bs207, _ = testMarshalErr(&v207v1, h, t, "-")
	v207v2 = nil
	testUnmarshalErr(&v207v2, bs207, h, t, "-")
	testDeepEqualErr(v207v1, v207v2, t, "-")

	v208v1 := map[int8]int8{10: 10}
	bs208, _ := testMarshalErr(v208v1, h, t, "-")
	v208v2 := make(map[int8]int8)
	testUnmarshalErr(v208v2, bs208, h, t, "-")
	testDeepEqualErr(v208v1, v208v2, t, "-")
	bs208, _ = testMarshalErr(&v208v1, h, t, "-")
	v208v2 = nil
	testUnmarshalErr(&v208v2, bs208, h, t, "-")
	testDeepEqualErr(v208v1, v208v2, t, "-")

	v209v1 := map[int8]int16{10: 10}
	bs209, _ := testMarshalErr(v209v1, h, t, "-")
	v209v2 := make(map[int8]int16)
	testUnmarshalErr(v209v2, bs209, h, t, "-")
	testDeepEqualErr(v209v1, v209v2, t, "-")
	bs209, _ = testMarshalErr(&v209v1, h, t, "-")
	v209v2 = nil
	testUnmarshalErr(&v209v2, bs209, h, t, "-")
	testDeepEqualErr(v209v1, v209v2, t, "-")

	v210v1 := map[int8]int32{10: 10}
	bs210, _ := testMarshalErr(v210v1, h, t, "-")
	v210v2 := make(map[int8]int32)
	testUnmarshalErr(v210v2, bs210, h, t, "-")
	testDeepEqualErr(v210v1, v210v2, t, "-")
	bs210, _ = testMarshalErr(&v210v1, h, t, "-")
	v210v2 = nil
	testUnmarshalErr(&v210v2, bs210, h, t, "-")
	testDeepEqualErr(v210v1, v210v2, t, "-")

	v211v1 := map[int8]int64{10: 10}
	bs211, _ := testMarshalErr(v211v1, h, t, "-")
	v211v2 := make(map[int8]int64)
	testUnmarshalErr(v211v2, bs211, h, t, "-")
	testDeepEqualErr(v211v1, v211v2, t, "-")
	bs211, _ = testMarshalErr(&v211v1, h, t, "-")
	v211v2 = nil
	testUnmarshalErr(&v211v2, bs211, h, t, "-")
	testDeepEqualErr(v211v1, v211v2, t, "-")

	v212v1 := map[int8]float32{10: 10.1}
	bs212, _ := testMarshalErr(v212v1, h, t, "-")
	v212v2 := make(map[int8]float32)
	testUnmarshalErr(v212v2, bs212, h, t, "-")
	testDeepEqualErr(v212v1, v212v2, t, "-")
	bs212, _ = testMarshalErr(&v212v1, h, t, "-")
	v212v2 = nil
	testUnmarshalErr(&v212v2, bs212, h, t, "-")
	testDeepEqualErr(v212v1, v212v2, t, "-")

	v213v1 := map[int8]float64{10: 10.1}
	bs213, _ := testMarshalErr(v213v1, h, t, "-")
	v213v2 := make(map[int8]float64)
	testUnmarshalErr(v213v2, bs213, h, t, "-")
	testDeepEqualErr(v213v1, v213v2, t, "-")
	bs213, _ = testMarshalErr(&v213v1, h, t, "-")
	v213v2 = nil
	testUnmarshalErr(&v213v2, bs213, h, t, "-")
	testDeepEqualErr(v213v1, v213v2, t, "-")

	v214v1 := map[int8]bool{10: true}
	bs214, _ := testMarshalErr(v214v1, h, t, "-")
	v214v2 := make(map[int8]bool)
	testUnmarshalErr(v214v2, bs214, h, t, "-")
	testDeepEqualErr(v214v1, v214v2, t, "-")
	bs214, _ = testMarshalErr(&v214v1, h, t, "-")
	v214v2 = nil
	testUnmarshalErr(&v214v2, bs214, h, t, "-")
	testDeepEqualErr(v214v1, v214v2, t, "-")

	v217v1 := map[int16]interface{}{10: "string-is-an-interface"}
	bs217, _ := testMarshalErr(v217v1, h, t, "-")
	v217v2 := make(map[int16]interface{})
	testUnmarshalErr(v217v2, bs217, h, t, "-")
	testDeepEqualErr(v217v1, v217v2, t, "-")
	bs217, _ = testMarshalErr(&v217v1, h, t, "-")
	v217v2 = nil
	testUnmarshalErr(&v217v2, bs217, h, t, "-")
	testDeepEqualErr(v217v1, v217v2, t, "-")

	v218v1 := map[int16]string{10: "some-string"}
	bs218, _ := testMarshalErr(v218v1, h, t, "-")
	v218v2 := make(map[int16]string)
	testUnmarshalErr(v218v2, bs218, h, t, "-")
	testDeepEqualErr(v218v1, v218v2, t, "-")
	bs218, _ = testMarshalErr(&v218v1, h, t, "-")
	v218v2 = nil
	testUnmarshalErr(&v218v2, bs218, h, t, "-")
	testDeepEqualErr(v218v1, v218v2, t, "-")

	v219v1 := map[int16]uint{10: 10}
	bs219, _ := testMarshalErr(v219v1, h, t, "-")
	v219v2 := make(map[int16]uint)
	testUnmarshalErr(v219v2, bs219, h, t, "-")
	testDeepEqualErr(v219v1, v219v2, t, "-")
	bs219, _ = testMarshalErr(&v219v1, h, t, "-")
	v219v2 = nil
	testUnmarshalErr(&v219v2, bs219, h, t, "-")
	testDeepEqualErr(v219v1, v219v2, t, "-")

	v220v1 := map[int16]uint8{10: 10}
	bs220, _ := testMarshalErr(v220v1, h, t, "-")
	v220v2 := make(map[int16]uint8)
	testUnmarshalErr(v220v2, bs220, h, t, "-")
	testDeepEqualErr(v220v1, v220v2, t, "-")
	bs220, _ = testMarshalErr(&v220v1, h, t, "-")
	v220v2 = nil
	testUnmarshalErr(&v220v2, bs220, h, t, "-")
	testDeepEqualErr(v220v1, v220v2, t, "-")

	v221v1 := map[int16]uint16{10: 10}
	bs221, _ := testMarshalErr(v221v1, h, t, "-")
	v221v2 := make(map[int16]uint16)
	testUnmarshalErr(v221v2, bs221, h, t, "-")
	testDeepEqualErr(v221v1, v221v2, t, "-")
	bs221, _ = testMarshalErr(&v221v1, h, t, "-")
	v221v2 = nil
	testUnmarshalErr(&v221v2, bs221, h, t, "-")
	testDeepEqualErr(v221v1, v221v2, t, "-")

	v222v1 := map[int16]uint32{10: 10}
	bs222, _ := testMarshalErr(v222v1, h, t, "-")
	v222v2 := make(map[int16]uint32)
	testUnmarshalErr(v222v2, bs222, h, t, "-")
	testDeepEqualErr(v222v1, v222v2, t, "-")
	bs222, _ = testMarshalErr(&v222v1, h, t, "-")
	v222v2 = nil
	testUnmarshalErr(&v222v2, bs222, h, t, "-")
	testDeepEqualErr(v222v1, v222v2, t, "-")

	v223v1 := map[int16]uint64{10: 10}
	bs223, _ := testMarshalErr(v223v1, h, t, "-")
	v223v2 := make(map[int16]uint64)
	testUnmarshalErr(v223v2, bs223, h, t, "-")
	testDeepEqualErr(v223v1, v223v2, t, "-")
	bs223, _ = testMarshalErr(&v223v1, h, t, "-")
	v223v2 = nil
	testUnmarshalErr(&v223v2, bs223, h, t, "-")
	testDeepEqualErr(v223v1, v223v2, t, "-")

	v224v1 := map[int16]uintptr{10: 10}
	bs224, _ := testMarshalErr(v224v1, h, t, "-")
	v224v2 := make(map[int16]uintptr)
	testUnmarshalErr(v224v2, bs224, h, t, "-")
	testDeepEqualErr(v224v1, v224v2, t, "-")
	bs224, _ = testMarshalErr(&v224v1, h, t, "-")
	v224v2 = nil
	testUnmarshalErr(&v224v2, bs224, h, t, "-")
	testDeepEqualErr(v224v1, v224v2, t, "-")

	v225v1 := map[int16]int{10: 10}
	bs225, _ := testMarshalErr(v225v1, h, t, "-")
	v225v2 := make(map[int16]int)
	testUnmarshalErr(v225v2, bs225, h, t, "-")
	testDeepEqualErr(v225v1, v225v2, t, "-")
	bs225, _ = testMarshalErr(&v225v1, h, t, "-")
	v225v2 = nil
	testUnmarshalErr(&v225v2, bs225, h, t, "-")
	testDeepEqualErr(v225v1, v225v2, t, "-")

	v226v1 := map[int16]int8{10: 10}
	bs226, _ := testMarshalErr(v226v1, h, t, "-")
	v226v2 := make(map[int16]int8)
	testUnmarshalErr(v226v2, bs226, h, t, "-")
	testDeepEqualErr(v226v1, v226v2, t, "-")
	bs226, _ = testMarshalErr(&v226v1, h, t, "-")
	v226v2 = nil
	testUnmarshalErr(&v226v2, bs226, h, t, "-")
	testDeepEqualErr(v226v1, v226v2, t, "-")

	v227v1 := map[int16]int16{10: 10}
	bs227, _ := testMarshalErr(v227v1, h, t, "-")
	v227v2 := make(map[int16]int16)
	testUnmarshalErr(v227v2, bs227, h, t, "-")
	testDeepEqualErr(v227v1, v227v2, t, "-")
	bs227, _ = testMarshalErr(&v227v1, h, t, "-")
	v227v2 = nil
	testUnmarshalErr(&v227v2, bs227, h, t, "-")
	testDeepEqualErr(v227v1, v227v2, t, "-")

	v228v1 := map[int16]int32{10: 10}
	bs228, _ := testMarshalErr(v228v1, h, t, "-")
	v228v2 := make(map[int16]int32)
	testUnmarshalErr(v228v2, bs228, h, t, "-")
	testDeepEqualErr(v228v1, v228v2, t, "-")
	bs228, _ = testMarshalErr(&v228v1, h, t, "-")
	v228v2 = nil
	testUnmarshalErr(&v228v2, bs228, h, t, "-")
	testDeepEqualErr(v228v1, v228v2, t, "-")

	v229v1 := map[int16]int64{10: 10}
	bs229, _ := testMarshalErr(v229v1, h, t, "-")
	v229v2 := make(map[int16]int64)
	testUnmarshalErr(v229v2, bs229, h, t, "-")
	testDeepEqualErr(v229v1, v229v2, t, "-")
	bs229, _ = testMarshalErr(&v229v1, h, t, "-")
	v229v2 = nil
	testUnmarshalErr(&v229v2, bs229, h, t, "-")
	testDeepEqualErr(v229v1, v229v2, t, "-")

	v230v1 := map[int16]float32{10: 10.1}
	bs230, _ := testMarshalErr(v230v1, h, t, "-")
	v230v2 := make(map[int16]float32)
	testUnmarshalErr(v230v2, bs230, h, t, "-")
	testDeepEqualErr(v230v1, v230v2, t, "-")
	bs230, _ = testMarshalErr(&v230v1, h, t, "-")
	v230v2 = nil
	testUnmarshalErr(&v230v2, bs230, h, t, "-")
	testDeepEqualErr(v230v1, v230v2, t, "-")

	v231v1 := map[int16]float64{10: 10.1}
	bs231, _ := testMarshalErr(v231v1, h, t, "-")
	v231v2 := make(map[int16]float64)
	testUnmarshalErr(v231v2, bs231, h, t, "-")
	testDeepEqualErr(v231v1, v231v2, t, "-")
	bs231, _ = testMarshalErr(&v231v1, h, t, "-")
	v231v2 = nil
	testUnmarshalErr(&v231v2, bs231, h, t, "-")
	testDeepEqualErr(v231v1, v231v2, t, "-")

	v232v1 := map[int16]bool{10: true}
	bs232, _ := testMarshalErr(v232v1, h, t, "-")
	v232v2 := make(map[int16]bool)
	testUnmarshalErr(v232v2, bs232, h, t, "-")
	testDeepEqualErr(v232v1, v232v2, t, "-")
	bs232, _ = testMarshalErr(&v232v1, h, t, "-")
	v232v2 = nil
	testUnmarshalErr(&v232v2, bs232, h, t, "-")
	testDeepEqualErr(v232v1, v232v2, t, "-")

	v235v1 := map[int32]interface{}{10: "string-is-an-interface"}
	bs235, _ := testMarshalErr(v235v1, h, t, "-")
	v235v2 := make(map[int32]interface{})
	testUnmarshalErr(v235v2, bs235, h, t, "-")
	testDeepEqualErr(v235v1, v235v2, t, "-")
	bs235, _ = testMarshalErr(&v235v1, h, t, "-")
	v235v2 = nil
	testUnmarshalErr(&v235v2, bs235, h, t, "-")
	testDeepEqualErr(v235v1, v235v2, t, "-")

	v236v1 := map[int32]string{10: "some-string"}
	bs236, _ := testMarshalErr(v236v1, h, t, "-")
	v236v2 := make(map[int32]string)
	testUnmarshalErr(v236v2, bs236, h, t, "-")
	testDeepEqualErr(v236v1, v236v2, t, "-")
	bs236, _ = testMarshalErr(&v236v1, h, t, "-")
	v236v2 = nil
	testUnmarshalErr(&v236v2, bs236, h, t, "-")
	testDeepEqualErr(v236v1, v236v2, t, "-")

	v237v1 := map[int32]uint{10: 10}
	bs237, _ := testMarshalErr(v237v1, h, t, "-")
	v237v2 := make(map[int32]uint)
	testUnmarshalErr(v237v2, bs237, h, t, "-")
	testDeepEqualErr(v237v1, v237v2, t, "-")
	bs237, _ = testMarshalErr(&v237v1, h, t, "-")
	v237v2 = nil
	testUnmarshalErr(&v237v2, bs237, h, t, "-")
	testDeepEqualErr(v237v1, v237v2, t, "-")

	v238v1 := map[int32]uint8{10: 10}
	bs238, _ := testMarshalErr(v238v1, h, t, "-")
	v238v2 := make(map[int32]uint8)
	testUnmarshalErr(v238v2, bs238, h, t, "-")
	testDeepEqualErr(v238v1, v238v2, t, "-")
	bs238, _ = testMarshalErr(&v238v1, h, t, "-")
	v238v2 = nil
	testUnmarshalErr(&v238v2, bs238, h, t, "-")
	testDeepEqualErr(v238v1, v238v2, t, "-")

	v239v1 := map[int32]uint16{10: 10}
	bs239, _ := testMarshalErr(v239v1, h, t, "-")
	v239v2 := make(map[int32]uint16)
	testUnmarshalErr(v239v2, bs239, h, t, "-")
	testDeepEqualErr(v239v1, v239v2, t, "-")
	bs239, _ = testMarshalErr(&v239v1, h, t, "-")
	v239v2 = nil
	testUnmarshalErr(&v239v2, bs239, h, t, "-")
	testDeepEqualErr(v239v1, v239v2, t, "-")

	v240v1 := map[int32]uint32{10: 10}
	bs240, _ := testMarshalErr(v240v1, h, t, "-")
	v240v2 := make(map[int32]uint32)
	testUnmarshalErr(v240v2, bs240, h, t, "-")
	testDeepEqualErr(v240v1, v240v2, t, "-")
	bs240, _ = testMarshalErr(&v240v1, h, t, "-")
	v240v2 = nil
	testUnmarshalErr(&v240v2, bs240, h, t, "-")
	testDeepEqualErr(v240v1, v240v2, t, "-")

	v241v1 := map[int32]uint64{10: 10}
	bs241, _ := testMarshalErr(v241v1, h, t, "-")
	v241v2 := make(map[int32]uint64)
	testUnmarshalErr(v241v2, bs241, h, t, "-")
	testDeepEqualErr(v241v1, v241v2, t, "-")
	bs241, _ = testMarshalErr(&v241v1, h, t, "-")
	v241v2 = nil
	testUnmarshalErr(&v241v2, bs241, h, t, "-")
	testDeepEqualErr(v241v1, v241v2, t, "-")

	v242v1 := map[int32]uintptr{10: 10}
	bs242, _ := testMarshalErr(v242v1, h, t, "-")
	v242v2 := make(map[int32]uintptr)
	testUnmarshalErr(v242v2, bs242, h, t, "-")
	testDeepEqualErr(v242v1, v242v2, t, "-")
	bs242, _ = testMarshalErr(&v242v1, h, t, "-")
	v242v2 = nil
	testUnmarshalErr(&v242v2, bs242, h, t, "-")
	testDeepEqualErr(v242v1, v242v2, t, "-")

	v243v1 := map[int32]int{10: 10}
	bs243, _ := testMarshalErr(v243v1, h, t, "-")
	v243v2 := make(map[int32]int)
	testUnmarshalErr(v243v2, bs243, h, t, "-")
	testDeepEqualErr(v243v1, v243v2, t, "-")
	bs243, _ = testMarshalErr(&v243v1, h, t, "-")
	v243v2 = nil
	testUnmarshalErr(&v243v2, bs243, h, t, "-")
	testDeepEqualErr(v243v1, v243v2, t, "-")

	v244v1 := map[int32]int8{10: 10}
	bs244, _ := testMarshalErr(v244v1, h, t, "-")
	v244v2 := make(map[int32]int8)
	testUnmarshalErr(v244v2, bs244, h, t, "-")
	testDeepEqualErr(v244v1, v244v2, t, "-")
	bs244, _ = testMarshalErr(&v244v1, h, t, "-")
	v244v2 = nil
	testUnmarshalErr(&v244v2, bs244, h, t, "-")
	testDeepEqualErr(v244v1, v244v2, t, "-")

	v245v1 := map[int32]int16{10: 10}
	bs245, _ := testMarshalErr(v245v1, h, t, "-")
	v245v2 := make(map[int32]int16)
	testUnmarshalErr(v245v2, bs245, h, t, "-")
	testDeepEqualErr(v245v1, v245v2, t, "-")
	bs245, _ = testMarshalErr(&v245v1, h, t, "-")
	v245v2 = nil
	testUnmarshalErr(&v245v2, bs245, h, t, "-")
	testDeepEqualErr(v245v1, v245v2, t, "-")

	v246v1 := map[int32]int32{10: 10}
	bs246, _ := testMarshalErr(v246v1, h, t, "-")
	v246v2 := make(map[int32]int32)
	testUnmarshalErr(v246v2, bs246, h, t, "-")
	testDeepEqualErr(v246v1, v246v2, t, "-")
	bs246, _ = testMarshalErr(&v246v1, h, t, "-")
	v246v2 = nil
	testUnmarshalErr(&v246v2, bs246, h, t, "-")
	testDeepEqualErr(v246v1, v246v2, t, "-")

	v247v1 := map[int32]int64{10: 10}
	bs247, _ := testMarshalErr(v247v1, h, t, "-")
	v247v2 := make(map[int32]int64)
	testUnmarshalErr(v247v2, bs247, h, t, "-")
	testDeepEqualErr(v247v1, v247v2, t, "-")
	bs247, _ = testMarshalErr(&v247v1, h, t, "-")
	v247v2 = nil
	testUnmarshalErr(&v247v2, bs247, h, t, "-")
	testDeepEqualErr(v247v1, v247v2, t, "-")

	v248v1 := map[int32]float32{10: 10.1}
	bs248, _ := testMarshalErr(v248v1, h, t, "-")
	v248v2 := make(map[int32]float32)
	testUnmarshalErr(v248v2, bs248, h, t, "-")
	testDeepEqualErr(v248v1, v248v2, t, "-")
	bs248, _ = testMarshalErr(&v248v1, h, t, "-")
	v248v2 = nil
	testUnmarshalErr(&v248v2, bs248, h, t, "-")
	testDeepEqualErr(v248v1, v248v2, t, "-")

	v249v1 := map[int32]float64{10: 10.1}
	bs249, _ := testMarshalErr(v249v1, h, t, "-")
	v249v2 := make(map[int32]float64)
	testUnmarshalErr(v249v2, bs249, h, t, "-")
	testDeepEqualErr(v249v1, v249v2, t, "-")
	bs249, _ = testMarshalErr(&v249v1, h, t, "-")
	v249v2 = nil
	testUnmarshalErr(&v249v2, bs249, h, t, "-")
	testDeepEqualErr(v249v1, v249v2, t, "-")

	v250v1 := map[int32]bool{10: true}
	bs250, _ := testMarshalErr(v250v1, h, t, "-")
	v250v2 := make(map[int32]bool)
	testUnmarshalErr(v250v2, bs250, h, t, "-")
	testDeepEqualErr(v250v1, v250v2, t, "-")
	bs250, _ = testMarshalErr(&v250v1, h, t, "-")
	v250v2 = nil
	testUnmarshalErr(&v250v2, bs250, h, t, "-")
	testDeepEqualErr(v250v1, v250v2, t, "-")

	v253v1 := map[int64]interface{}{10: "string-is-an-interface"}
	bs253, _ := testMarshalErr(v253v1, h, t, "-")
	v253v2 := make(map[int64]interface{})
	testUnmarshalErr(v253v2, bs253, h, t, "-")
	testDeepEqualErr(v253v1, v253v2, t, "-")
	bs253, _ = testMarshalErr(&v253v1, h, t, "-")
	v253v2 = nil
	testUnmarshalErr(&v253v2, bs253, h, t, "-")
	testDeepEqualErr(v253v1, v253v2, t, "-")

	v254v1 := map[int64]string{10: "some-string"}
	bs254, _ := testMarshalErr(v254v1, h, t, "-")
	v254v2 := make(map[int64]string)
	testUnmarshalErr(v254v2, bs254, h, t, "-")
	testDeepEqualErr(v254v1, v254v2, t, "-")
	bs254, _ = testMarshalErr(&v254v1, h, t, "-")
	v254v2 = nil
	testUnmarshalErr(&v254v2, bs254, h, t, "-")
	testDeepEqualErr(v254v1, v254v2, t, "-")

	v255v1 := map[int64]uint{10: 10}
	bs255, _ := testMarshalErr(v255v1, h, t, "-")
	v255v2 := make(map[int64]uint)
	testUnmarshalErr(v255v2, bs255, h, t, "-")
	testDeepEqualErr(v255v1, v255v2, t, "-")
	bs255, _ = testMarshalErr(&v255v1, h, t, "-")
	v255v2 = nil
	testUnmarshalErr(&v255v2, bs255, h, t, "-")
	testDeepEqualErr(v255v1, v255v2, t, "-")

	v256v1 := map[int64]uint8{10: 10}
	bs256, _ := testMarshalErr(v256v1, h, t, "-")
	v256v2 := make(map[int64]uint8)
	testUnmarshalErr(v256v2, bs256, h, t, "-")
	testDeepEqualErr(v256v1, v256v2, t, "-")
	bs256, _ = testMarshalErr(&v256v1, h, t, "-")
	v256v2 = nil
	testUnmarshalErr(&v256v2, bs256, h, t, "-")
	testDeepEqualErr(v256v1, v256v2, t, "-")

	v257v1 := map[int64]uint16{10: 10}
	bs257, _ := testMarshalErr(v257v1, h, t, "-")
	v257v2 := make(map[int64]uint16)
	testUnmarshalErr(v257v2, bs257, h, t, "-")
	testDeepEqualErr(v257v1, v257v2, t, "-")
	bs257, _ = testMarshalErr(&v257v1, h, t, "-")
	v257v2 = nil
	testUnmarshalErr(&v257v2, bs257, h, t, "-")
	testDeepEqualErr(v257v1, v257v2, t, "-")

	v258v1 := map[int64]uint32{10: 10}
	bs258, _ := testMarshalErr(v258v1, h, t, "-")
	v258v2 := make(map[int64]uint32)
	testUnmarshalErr(v258v2, bs258, h, t, "-")
	testDeepEqualErr(v258v1, v258v2, t, "-")
	bs258, _ = testMarshalErr(&v258v1, h, t, "-")
	v258v2 = nil
	testUnmarshalErr(&v258v2, bs258, h, t, "-")
	testDeepEqualErr(v258v1, v258v2, t, "-")

	v259v1 := map[int64]uint64{10: 10}
	bs259, _ := testMarshalErr(v259v1, h, t, "-")
	v259v2 := make(map[int64]uint64)
	testUnmarshalErr(v259v2, bs259, h, t, "-")
	testDeepEqualErr(v259v1, v259v2, t, "-")
	bs259, _ = testMarshalErr(&v259v1, h, t, "-")
	v259v2 = nil
	testUnmarshalErr(&v259v2, bs259, h, t, "-")
	testDeepEqualErr(v259v1, v259v2, t, "-")

	v260v1 := map[int64]uintptr{10: 10}
	bs260, _ := testMarshalErr(v260v1, h, t, "-")
	v260v2 := make(map[int64]uintptr)
	testUnmarshalErr(v260v2, bs260, h, t, "-")
	testDeepEqualErr(v260v1, v260v2, t, "-")
	bs260, _ = testMarshalErr(&v260v1, h, t, "-")
	v260v2 = nil
	testUnmarshalErr(&v260v2, bs260, h, t, "-")
	testDeepEqualErr(v260v1, v260v2, t, "-")

	v261v1 := map[int64]int{10: 10}
	bs261, _ := testMarshalErr(v261v1, h, t, "-")
	v261v2 := make(map[int64]int)
	testUnmarshalErr(v261v2, bs261, h, t, "-")
	testDeepEqualErr(v261v1, v261v2, t, "-")
	bs261, _ = testMarshalErr(&v261v1, h, t, "-")
	v261v2 = nil
	testUnmarshalErr(&v261v2, bs261, h, t, "-")
	testDeepEqualErr(v261v1, v261v2, t, "-")

	v262v1 := map[int64]int8{10: 10}
	bs262, _ := testMarshalErr(v262v1, h, t, "-")
	v262v2 := make(map[int64]int8)
	testUnmarshalErr(v262v2, bs262, h, t, "-")
	testDeepEqualErr(v262v1, v262v2, t, "-")
	bs262, _ = testMarshalErr(&v262v1, h, t, "-")
	v262v2 = nil
	testUnmarshalErr(&v262v2, bs262, h, t, "-")
	testDeepEqualErr(v262v1, v262v2, t, "-")

	v263v1 := map[int64]int16{10: 10}
	bs263, _ := testMarshalErr(v263v1, h, t, "-")
	v263v2 := make(map[int64]int16)
	testUnmarshalErr(v263v2, bs263, h, t, "-")
	testDeepEqualErr(v263v1, v263v2, t, "-")
	bs263, _ = testMarshalErr(&v263v1, h, t, "-")
	v263v2 = nil
	testUnmarshalErr(&v263v2, bs263, h, t, "-")
	testDeepEqualErr(v263v1, v263v2, t, "-")

	v264v1 := map[int64]int32{10: 10}
	bs264, _ := testMarshalErr(v264v1, h, t, "-")
	v264v2 := make(map[int64]int32)
	testUnmarshalErr(v264v2, bs264, h, t, "-")
	testDeepEqualErr(v264v1, v264v2, t, "-")
	bs264, _ = testMarshalErr(&v264v1, h, t, "-")
	v264v2 = nil
	testUnmarshalErr(&v264v2, bs264, h, t, "-")
	testDeepEqualErr(v264v1, v264v2, t, "-")

	v265v1 := map[int64]int64{10: 10}
	bs265, _ := testMarshalErr(v265v1, h, t, "-")
	v265v2 := make(map[int64]int64)
	testUnmarshalErr(v265v2, bs265, h, t, "-")
	testDeepEqualErr(v265v1, v265v2, t, "-")
	bs265, _ = testMarshalErr(&v265v1, h, t, "-")
	v265v2 = nil
	testUnmarshalErr(&v265v2, bs265, h, t, "-")
	testDeepEqualErr(v265v1, v265v2, t, "-")

	v266v1 := map[int64]float32{10: 10.1}
	bs266, _ := testMarshalErr(v266v1, h, t, "-")
	v266v2 := make(map[int64]float32)
	testUnmarshalErr(v266v2, bs266, h, t, "-")
	testDeepEqualErr(v266v1, v266v2, t, "-")
	bs266, _ = testMarshalErr(&v266v1, h, t, "-")
	v266v2 = nil
	testUnmarshalErr(&v266v2, bs266, h, t, "-")
	testDeepEqualErr(v266v1, v266v2, t, "-")

	v267v1 := map[int64]float64{10: 10.1}
	bs267, _ := testMarshalErr(v267v1, h, t, "-")
	v267v2 := make(map[int64]float64)
	testUnmarshalErr(v267v2, bs267, h, t, "-")
	testDeepEqualErr(v267v1, v267v2, t, "-")
	bs267, _ = testMarshalErr(&v267v1, h, t, "-")
	v267v2 = nil
	testUnmarshalErr(&v267v2, bs267, h, t, "-")
	testDeepEqualErr(v267v1, v267v2, t, "-")

	v268v1 := map[int64]bool{10: true}
	bs268, _ := testMarshalErr(v268v1, h, t, "-")
	v268v2 := make(map[int64]bool)
	testUnmarshalErr(v268v2, bs268, h, t, "-")
	testDeepEqualErr(v268v1, v268v2, t, "-")
	bs268, _ = testMarshalErr(&v268v1, h, t, "-")
	v268v2 = nil
	testUnmarshalErr(&v268v2, bs268, h, t, "-")
	testDeepEqualErr(v268v1, v268v2, t, "-")

	v271v1 := map[bool]interface{}{true: "string-is-an-interface"}
	bs271, _ := testMarshalErr(v271v1, h, t, "-")
	v271v2 := make(map[bool]interface{})
	testUnmarshalErr(v271v2, bs271, h, t, "-")
	testDeepEqualErr(v271v1, v271v2, t, "-")
	bs271, _ = testMarshalErr(&v271v1, h, t, "-")
	v271v2 = nil
	testUnmarshalErr(&v271v2, bs271, h, t, "-")
	testDeepEqualErr(v271v1, v271v2, t, "-")

	v272v1 := map[bool]string{true: "some-string"}
	bs272, _ := testMarshalErr(v272v1, h, t, "-")
	v272v2 := make(map[bool]string)
	testUnmarshalErr(v272v2, bs272, h, t, "-")
	testDeepEqualErr(v272v1, v272v2, t, "-")
	bs272, _ = testMarshalErr(&v272v1, h, t, "-")
	v272v2 = nil
	testUnmarshalErr(&v272v2, bs272, h, t, "-")
	testDeepEqualErr(v272v1, v272v2, t, "-")

	v273v1 := map[bool]uint{true: 10}
	bs273, _ := testMarshalErr(v273v1, h, t, "-")
	v273v2 := make(map[bool]uint)
	testUnmarshalErr(v273v2, bs273, h, t, "-")
	testDeepEqualErr(v273v1, v273v2, t, "-")
	bs273, _ = testMarshalErr(&v273v1, h, t, "-")
	v273v2 = nil
	testUnmarshalErr(&v273v2, bs273, h, t, "-")
	testDeepEqualErr(v273v1, v273v2, t, "-")

	v274v1 := map[bool]uint8{true: 10}
	bs274, _ := testMarshalErr(v274v1, h, t, "-")
	v274v2 := make(map[bool]uint8)
	testUnmarshalErr(v274v2, bs274, h, t, "-")
	testDeepEqualErr(v274v1, v274v2, t, "-")
	bs274, _ = testMarshalErr(&v274v1, h, t, "-")
	v274v2 = nil
	testUnmarshalErr(&v274v2, bs274, h, t, "-")
	testDeepEqualErr(v274v1, v274v2, t, "-")

	v275v1 := map[bool]uint16{true: 10}
	bs275, _ := testMarshalErr(v275v1, h, t, "-")
	v275v2 := make(map[bool]uint16)
	testUnmarshalErr(v275v2, bs275, h, t, "-")
	testDeepEqualErr(v275v1, v275v2, t, "-")
	bs275, _ = testMarshalErr(&v275v1, h, t, "-")
	v275v2 = nil
	testUnmarshalErr(&v275v2, bs275, h, t, "-")
	testDeepEqualErr(v275v1, v275v2, t, "-")

	v276v1 := map[bool]uint32{true: 10}
	bs276, _ := testMarshalErr(v276v1, h, t, "-")
	v276v2 := make(map[bool]uint32)
	testUnmarshalErr(v276v2, bs276, h, t, "-")
	testDeepEqualErr(v276v1, v276v2, t, "-")
	bs276, _ = testMarshalErr(&v276v1, h, t, "-")
	v276v2 = nil
	testUnmarshalErr(&v276v2, bs276, h, t, "-")
	testDeepEqualErr(v276v1, v276v2, t, "-")

	v277v1 := map[bool]uint64{true: 10}
	bs277, _ := testMarshalErr(v277v1, h, t, "-")
	v277v2 := make(map[bool]uint64)
	testUnmarshalErr(v277v2, bs277, h, t, "-")
	testDeepEqualErr(v277v1, v277v2, t, "-")
	bs277, _ = testMarshalErr(&v277v1, h, t, "-")
	v277v2 = nil
	testUnmarshalErr(&v277v2, bs277, h, t, "-")
	testDeepEqualErr(v277v1, v277v2, t, "-")

	v278v1 := map[bool]uintptr{true: 10}
	bs278, _ := testMarshalErr(v278v1, h, t, "-")
	v278v2 := make(map[bool]uintptr)
	testUnmarshalErr(v278v2, bs278, h, t, "-")
	testDeepEqualErr(v278v1, v278v2, t, "-")
	bs278, _ = testMarshalErr(&v278v1, h, t, "-")
	v278v2 = nil
	testUnmarshalErr(&v278v2, bs278, h, t, "-")
	testDeepEqualErr(v278v1, v278v2, t, "-")

	v279v1 := map[bool]int{true: 10}
	bs279, _ := testMarshalErr(v279v1, h, t, "-")
	v279v2 := make(map[bool]int)
	testUnmarshalErr(v279v2, bs279, h, t, "-")
	testDeepEqualErr(v279v1, v279v2, t, "-")
	bs279, _ = testMarshalErr(&v279v1, h, t, "-")
	v279v2 = nil
	testUnmarshalErr(&v279v2, bs279, h, t, "-")
	testDeepEqualErr(v279v1, v279v2, t, "-")

	v280v1 := map[bool]int8{true: 10}
	bs280, _ := testMarshalErr(v280v1, h, t, "-")
	v280v2 := make(map[bool]int8)
	testUnmarshalErr(v280v2, bs280, h, t, "-")
	testDeepEqualErr(v280v1, v280v2, t, "-")
	bs280, _ = testMarshalErr(&v280v1, h, t, "-")
	v280v2 = nil
	testUnmarshalErr(&v280v2, bs280, h, t, "-")
	testDeepEqualErr(v280v1, v280v2, t, "-")

	v281v1 := map[bool]int16{true: 10}
	bs281, _ := testMarshalErr(v281v1, h, t, "-")
	v281v2 := make(map[bool]int16)
	testUnmarshalErr(v281v2, bs281, h, t, "-")
	testDeepEqualErr(v281v1, v281v2, t, "-")
	bs281, _ = testMarshalErr(&v281v1, h, t, "-")
	v281v2 = nil
	testUnmarshalErr(&v281v2, bs281, h, t, "-")
	testDeepEqualErr(v281v1, v281v2, t, "-")

	v282v1 := map[bool]int32{true: 10}
	bs282, _ := testMarshalErr(v282v1, h, t, "-")
	v282v2 := make(map[bool]int32)
	testUnmarshalErr(v282v2, bs282, h, t, "-")
	testDeepEqualErr(v282v1, v282v2, t, "-")
	bs282, _ = testMarshalErr(&v282v1, h, t, "-")
	v282v2 = nil
	testUnmarshalErr(&v282v2, bs282, h, t, "-")
	testDeepEqualErr(v282v1, v282v2, t, "-")

	v283v1 := map[bool]int64{true: 10}
	bs283, _ := testMarshalErr(v283v1, h, t, "-")
	v283v2 := make(map[bool]int64)
	testUnmarshalErr(v283v2, bs283, h, t, "-")
	testDeepEqualErr(v283v1, v283v2, t, "-")
	bs283, _ = testMarshalErr(&v283v1, h, t, "-")
	v283v2 = nil
	testUnmarshalErr(&v283v2, bs283, h, t, "-")
	testDeepEqualErr(v283v1, v283v2, t, "-")

	v284v1 := map[bool]float32{true: 10.1}
	bs284, _ := testMarshalErr(v284v1, h, t, "-")
	v284v2 := make(map[bool]float32)
	testUnmarshalErr(v284v2, bs284, h, t, "-")
	testDeepEqualErr(v284v1, v284v2, t, "-")
	bs284, _ = testMarshalErr(&v284v1, h, t, "-")
	v284v2 = nil
	testUnmarshalErr(&v284v2, bs284, h, t, "-")
	testDeepEqualErr(v284v1, v284v2, t, "-")

	v285v1 := map[bool]float64{true: 10.1}
	bs285, _ := testMarshalErr(v285v1, h, t, "-")
	v285v2 := make(map[bool]float64)
	testUnmarshalErr(v285v2, bs285, h, t, "-")
	testDeepEqualErr(v285v1, v285v2, t, "-")
	bs285, _ = testMarshalErr(&v285v1, h, t, "-")
	v285v2 = nil
	testUnmarshalErr(&v285v2, bs285, h, t, "-")
	testDeepEqualErr(v285v1, v285v2, t, "-")

	v286v1 := map[bool]bool{true: true}
	bs286, _ := testMarshalErr(v286v1, h, t, "-")
	v286v2 := make(map[bool]bool)
	testUnmarshalErr(v286v2, bs286, h, t, "-")
	testDeepEqualErr(v286v1, v286v2, t, "-")
	bs286, _ = testMarshalErr(&v286v1, h, t, "-")
	v286v2 = nil
	testUnmarshalErr(&v286v2, bs286, h, t, "-")
	testDeepEqualErr(v286v1, v286v2, t, "-")

}

func doTestMammothMapsAndSlices(t *testing.T, h Handle) {
	doTestMammothSlices(t, h)
	doTestMammothMaps(t, h)
}
