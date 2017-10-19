// Copyright © 2017 The Kubicorn Authors
//
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

package test

import (
	"io/ioutil"
	"os"
)

var (
	TestPrivateRsa = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAsdM0zQDU7TJBWuawREBnmgd7wSIqvbuMqOs4Q1a8xjGeEie0
fvPn9ZvyGyQvVnxvOHUfdUET5cYWO0xuHy5AiZSJVqiN5DCpd2u9UlOlNU0iMj3V
Eh0GllrWD88i9Xk85e/3xYTwxpdJ7ncTIZzM9rXUCeidK8sQS2Ve4dQGMHMf8US5
Cp+1tK5y6bYPLsre1i9nWAjZNDtG2gp1TlkX6DLLfre24Es1bjVIAI+jbLIo4L7e
89Rps/CJmkAZJ00fmt3S5Enc1OvCBSuNb+uvYMW/2XU2M1LfEeIYGo0bbHLe4/qw
njKKK5B+H1CPTk/eJAx+hAn7WxxfEvGHLWsYIwIDAQABAoIBAQCpV9RBohgj5qb8
dRHJfXfr5FKDIxGG6/NQ7ef/oLtXFutMqMkn2Qi+Cgtus2/tMcUNA+S4Wggj2hdT
0z5PrVFCc9SyVQQDGiBYnJ6HpyZ+cv0s0Vt2y3N5ffm6xmypThKjenn/fNF6nZqH
YJg0e0lpbNEHuqDqko/q7ReFgc9/FJzpS8GnTGDr7kD2PNtyW8KlclTYHlfW1H54
XXa3DWKaFIiDwNBQU8Hs0FcngdnaLpgNqF9y4J6GCFTRuHczsyIZ5ATos+SboYM2
Mzz8hsAz3F04EFOvYYkctl6pUzxPRaI5wlJaV52zgoXe1KFXa1dbA3KrBrEb/fwh
RPKQHsapAoGBANzfnMcEWSIPBej/dz/s+p9ZO3xdCjshbPzR2yMn0Pst40KOHhYW
k35uM+WvEPv48a6Pp5pmGv0/SESaRjh5HQvxvduwdRKvttfXUWHk5UqD6jvDVRKH
6gshY6YfaZU0IG1k6yexXf1+ql0NyvPK8i5LtGHrD9m+2Jgrfu6yD/p3AoGBAM4a
9/TEt0Df8d4nCygKYV30qkWx5iZxS2G9vrJCNeNckynkDBCoKg6F0651FARe+3+L
quN1OUbB0VArRAfzZZPzITnI4XA9MZrv05HITKKRfOkoe69ssg5YMuRTPyn74sCV
ErFcyTpu1lCk7X59AmjYSZ364NrtxJhD3riJl461AoGADb9BM8XaglsrBAB6fJkU
VDyqjigATgPbk7TADeUZhbiqb2cHClrnXTQguMf3p6cr67B3Pw3h2idJKTPs8PDg
1PB736OQ9dPH7pExOIWVm9iwCH402k1pTL4MRLepy6aN6iEg3byVXAS5N8d2/UuB
XU5K8Nk/iE7vjjEO2m5svisCgYEAh1FFgrq05i8iCYzw0jUegCVmtaN7S7oOl/mP
/lFiOAhLxrEnCrieBDLxLBVKMyR5UuBMLlKEbGRMHKqLW/z9sAlswxeUi7BhpSvY
aFptlj6XGC2wJxjiPnDB2Q6e5d2unmpBf5k/tNGYfBIMq4M/1b5LdyGEB7kb3iyR
Se9sRhECgYEAq9G7kRjN2UmPMg1K0F1KmC3Q+jx1NPZIs0jj6EdYVi4hpTLmFrYT
O1KnEMD/m1FqgCdfJSB0T6/N6outIZoL9f/Uhcw92v62DIVZu2pq7KU0+k3Ngn98
8tbw4jK1D/hWSGx6a7dyWG2CfqxYE/xjxAWJmDtItwK38Mvl0cB1mC4=
-----END RSA PRIVATE KEY-----`)

	TestPublicRsa = []byte(`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCx0zTNANTtMkFa5rBEQGeaB3vBIiq9u4yo6zhDVrzGMZ4SJ7R+8+f1m/IbJC9WfG84dR91QRPlxhY7TG4fLkCJlIlWqI3kMKl3a71SU6U1TSIyPdUSHQaWWtYPzyL1eTzl7/fFhPDGl0nudxMhnMz2tdQJ6J0ryxBLZV7h1AYwcx/xRLkKn7W0rnLptg8uyt7WL2dYCNk0O0baCnVOWRfoMst+t7bgSzVuNUgAj6Nssijgvt7z1Gmz8ImaQBknTR+a3dLkSdzU68IFK41v669gxb/ZdTYzUt8R4hgajRtsct7j+rCeMoorkH4fUI9OT94kDH6ECftbHF8S8Yctaxgj kris@kris`)
)

func InitRsaTravis() {
	os.Mkdir("/home/travis/.ssh", 0700)
	ioutil.WriteFile("/home/travis/.ssh/id_rsa.pub", TestPublicRsa, 0600)
	ioutil.WriteFile("/home/travis/.ssh/id_rsa", TestPrivateRsa, 0600)
}
