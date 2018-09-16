// Copyright © 2018 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Some of the implementation of this package was borrowed from
// https://github.com/dspinhirne/netaddr-rb/tree/version-1.5.2

package util

import (
	"encoding/binary"
	"errors"
	"math"
	"net"
)

const (
	// LEFTMOST assigns subnets from the left-most bits of the super network.
	LEFTMOST = 0

	// CENTERMOST assigns subnets from the centre-most bits of the super network.
	CENTERMOST = 1

	// RIGHTMOST assigns subnets from the right-most bits of the super network.
	RIGHTMOST = 2
)

// AllocateRFC3531 allocates IPv4 subnets from a supernetwork using RFC 3531
// where super is the super network from which subnets will be allocated
// and size is the desired subnet mask.
//
// See https://tools.ietf.org/html/rfc3531 for more information.
func AllocateRFC3531(super string, size int, strategy int) ([]string, error) {

	_, cidr, err := net.ParseCIDR(super)
	if err != nil {
		return nil, errors.New("could not parse IP address")
	}
	superBits, ipBits := cidr.Mask.Size()
	subnetBits := size - superBits

	if subnetBits < 0 {
		return nil, errors.New("cannot allocate subnet larger than supernetwork")
	}

	if strategy == LEFTMOST {
		return rfc3531Leftmost(cidr, size, subnetBits, ipBits), nil
	} else if strategy == CENTERMOST {
		return rfc3531Centermost(cidr, size, subnetBits, ipBits), nil
	} else if strategy == RIGHTMOST {
		return rfc3531Rightmost(cidr, size, subnetBits, ipBits), nil
	} else {
		return nil, errors.New("no valid strategy was selected")
	}
}

func rfc3531Rightmost(super *net.IPNet, size int, subnetBits int, ipBits int) []string {
	leftMost := rfc3531Leftmost(super, size, subnetBits, ipBits)

	// Reverse the result of rfc3531Leftmost
	for i := 0; i < len(leftMost)/2; i++ {
		j := len(leftMost) - i - 1
		leftMost[i], leftMost[j] = leftMost[j], leftMost[i]
	}
	return leftMost
}

func rfc3531Leftmost(super *net.IPNet, size int, subnetBits int, ipBits int) []string {
	var list []string
	lShift := ipBits - size

	// Mirror binary counting using the left most bits
	for i := 0; i < (int(math.Pow(2, float64(subnetBits)))); i++ {
		shift := binaryMirror(i, subnetBits) << uint(lShift)
		baseInt := binary.BigEndian.Uint32(super.IP) | uint32(shift)
		base := make(net.IP, 4)
		binary.BigEndian.PutUint32(base, baseInt)
		subnet := net.IPNet{
			IP:   base,
			Mask: net.CIDRMask(size, ipBits)}
		list = append(list, subnet.String())
	}
	return list

}

func rfc3531Centermost(super *net.IPNet, size int, subnetBits int, ipBits int) []string {
	var list []string

	/*
	 		1.  The first round is to select only the middle bit (and if there is
	        an even number of bits  pick the bit following the center)
	*/
	round := 1
	netlShift := uint(ipBits - size)
	lShift := subnetBits / 2
	if subnetBits&1 == 0 {
		lShift--
	}
	uniques := make(map[int]bool)
	for bitCount := 1; bitCount <= subnetBits; bitCount++ {

		/*
			2.  Create all combinations using the selected bits that haven't yet
					been created.
		*/

		for i := 0; i < (int(math.Pow(2, float64(bitCount)))); i++ {
			shifted := i << uint(lShift)
			if !uniques[shifted] {
				baseInt := binary.BigEndian.Uint32(super.IP) | uint32(shifted)<<netlShift
				base := make(net.IP, 4)
				binary.BigEndian.PutUint32(base, baseInt)
				subnet := net.IPNet{
					IP:   base,
					Mask: net.CIDRMask(size, ipBits)}
				list = append(list, subnet.String())
				uniques[shifted] = true
			}
		}
		/*
		   3.  Start a new round by adding one more bit to the set.  In even
		       rounds add the preceding bit to the set.  In odd rounds add the
		       subsequent bit to the set.
		*/
		if round&1 == 0 {
			lShift--
		}
		round++

		/*
			4.  Repeat 2 and 3 until there are no more bits to consider.
		*/
	}

	return list
}

func binaryMirror(num int, bitCount int) int {
	mirror := 0
	for i := 0; i < bitCount; i++ {
		lsb := num & 1
		num = num >> 1
		mirror = (mirror << 1) | lsb
	}
	return mirror
}
