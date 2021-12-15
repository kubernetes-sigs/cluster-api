/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ignition

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
)

func TestRealUseCase(t *testing.T) {
	g := NewWithT(t)

	cloudData := []byte(`{
    "storage": {
      "files": [
        {
          "filesystem": "root",
          "path": "/etc/kubernetes/pki/ca.crt",
          "contents": {
            "source": "data:,-----BEGIN%20CERTIFICATE-----%0AMIIC6jCCAdKgAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl%0Acm5ldGVzMB4XDTIxMDkxNDExNTYyMVoXDTMxMDkxMjEyMDEyMVowFTETMBEGA1UE%0AAxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKpd%0AhNm74fN6MxrAlLkuRGVJIdrUP9D0JDm84yE71d8g22Bt%2BVrMXuaHthUWMsF0F%2FHF%0AaIo4zb7Rj8lgC5xnifrSaBFtd%2BdTRXe6kQHCzIvFhKpnOT3Q7NHnAzOXrkt8tOE0%0AnChh9wY%2BSQyTUZd5v5Mghsdi3vSzoQSLtrCD%2BNyMx600BWO8ME%2Bv4mqvnriT8xYs%0Atci45R4nzDauyJCawKQVvQl5Prl34hexptYIVbtVAt0Qm8Zqh2MTmKlIvnpr7kpl%0A7l%2FKvaRYZEgw58ge1vz91bOCuqDunRUQDtybJDayRUPyBhZHv6XP8QU%2FC2YR0GIE%0Ae8%2Fkszd1BvoT4likKz0CAwEAAaNFMEMwDgYDVR0PAQH%2FBAQDAgKkMBIGA1UdEwEB%0A%2FwQIMAYBAf8CAQAwHQYDVR0OBBYEFBGrJ3PFFKipZVwK7te%2BHE2z2fRjMA0GCSqG%0ASIb3DQEBCwUAA4IBAQA6SMrz10EoIhf2f%2BNOKp60TXhPnyFjd5YicPAo2b3zHAkJ%0AL05M1dQHTN626ue%2BuBQrWQ0MzZv9NNlIkhRtuf%2B6qmMi2Rq2UJWSavH5bm%2B2nxLl%0Amh1iaK%2BG8%2FmUxSWsTC%2BsexXMNNJAEM%2BwOQhcYd04ia%2BrmjuO7rI0gS8JCkW4%2Bh4B%0A0374kzTJrCZ%2BMhkyJqa0jVEW5fDUS02FwKP2Jf7nSWOP1T9e82tzwgIu6cHioU55%0AQg0raZj7neSdigRArT9488stSmwompwzpP6pwizcA2NLsjQx%2BbHT6x09VQrLuRiD%0AhVvcFSj2HhoXFTl2CJ1eYVzRpJhsvzjxZi6%2F8eIa%0A-----END%20CERTIFICATE-----%0A",
            "verification": {}
          },
          "mode": 416
        },
        {
          "filesystem": "root",
          "path": "/etc/kubernetes/pki/ca.key",
          "contents": {
            "source": "data:,-----BEGIN%20RSA%20PRIVATE%20KEY-----%0AMIIEogIBAAKCAQEAql2E2bvh83ozGsCUuS5EZUkh2tQ%2F0PQkObzjITvV3yDbYG35%0AWsxe5oe2FRYywXQX8cVoijjNvtGPyWALnGeJ%2BtJoEW1351NFd7qRAcLMi8WEqmc5%0APdDs0ecDM5euS3y04TScKGH3Bj5JDJNRl3m%2FkyCGx2Le9LOhBIu2sIP43IzHrTQF%0AY7wwT6%2Fiaq%2BeuJPzFiy1yLjlHifMNq7IkJrApBW9CXk%2BuXfiF7Gm1ghVu1UC3RCb%0AxmqHYxOYqUi%2BemvuSmXuX8q9pFhkSDDnyB7W%2FP3Vs4K6oO6dFRAO3JskNrJFQ%2FIG%0AFke%2Fpc%2FxBT8LZhHQYgR7z%2BSzN3UG%2BhPiWKQrPQIDAQABAoIBAFGSvc3TnHkMhfPF%0ASnDwqmclAUTaZEQU4lOTEd4T3HAeN2yQu9iyCq6vRIwMOPlQMTbeoxOr5zf697Ig%0Afu7A1Nx4asQNemAVCyos9sm1EGPMi51cF5h1tS88QdguRJJ4f9NlcXAUmEcxA6E1%0A2NeCwCweYuqNeNwKNosKqssSJdLT%2Fbm0kiXKE2h4chxz7oqcn1bjtC%2BIHOQIIZ77%0Ahiof3qgiSUJ6E9GOoHk9Y%2FoXTrTmHT0WvoODc53rGT42T8UydzHH6nQIIxz1Mt%2F9%0AoRWwUay2cNf3WsOc1K6OM3qxHNVz2Z7JMf23BgSXl4Illo1cFl2b0vc0e18bouSc%0ArYVsaIECgYEA0XO137fFSp3Aq6PAHucSOCdA9xXoWRnmPoU0sQfplg4Mmjf6%2FIny%0A1rphhMt4d2Aa0p5LInX8WkhkhyvUauFOHBQYxtonjWJ90NcrcP3tRI4Vt4H6XTgF%0AGb51Xgc3vb%2FtGZvPMdyE8oVkj8x7U%2FXls3%2B5wk2q2AJBDBVXRxyQPVUCgYEA0DoP%0AuIe9pofnwLTO17V7g6AniOTnF6Yai2I0Ly3Lbc74pTXodTFcG2NVtAJxQMYGpQri%0A4PfqagzTXLq1ccEa%2BpvcNlZlv7t4vu46KAVo44xwHdCc%2F7LLfS0B23oDXg540ZH9%0ACNZz5g%2BRbZuZQZyYjVIjK2%2BQmrrXF6P3GT5H9kkCgYAw%2BkrURqfW2%2B669C6vyz7i%0AbKNvY%2BsSMtE5W3LH1t7TXPOreF2zghqMBcdaAy5nU8zR5XwSUd6xye3gAerJF2hp%0AfnWQwmCvWhGrrTUWVfqOpl8Dq1w9QiVHMNdHJo7tSx0JePrJYRShlXm%2FeoR4TK7q%0A%2B3oXqovBuT02syLWmSJNhQKBgCr%2FYlGrjgj%2BVWfgrjmy2w%2BCGcfV5LZocWDI5Ze8%0AcB57t7J94EOa7rclGwRx4KsMeUDJb7Ie34QIo%2FipAWC9DHIljyKVUqt17egXT2EG%0ARPOAA4LUmibe59AwZArLNjjM6jv0Vnjlt8cQ%2FenRUKNQz9uW03ZbslORM2tJS3Ql%0A%2FTwpAoGAFBYQarNj%2B4ruEvGAENP2oECq9RIKReJ5C%2FhuaITFC7qP15YmjsoTyjiG%0AJIi1FLtpDsBDEyMkOfjIJrfdGkKcWoSxAM%2FV0smuF1lc2SJECh9ESHx5IVZVcuQg%0ASBxQDUUoeu0jy2Ugkr6%2B06q%2BbUt4PkCBiczdZz1RchHnw6pyid0%3D%0A-----END%20RSA%20PRIVATE%20KEY-----%0A",
            "verification": {}
          },
          "mode": 384
        }
      ]
    },
    "systemd": {
      "units": [
        {
          "contents": "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
          "enabled": true,
          "name": "kubeadm.service"
        }
      ]
    }
  }`)

	expectedCmds := []provisioning.Cmd{
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/ca.crt /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/etc/kubernetes/pki/ca.crt"}},
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/ca.key /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0600", "/etc/kubernetes/pki/ca.key"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/systemd/system/kubeadm.service /dev/stdin"}},
		{Cmd: "systemctl", Args: []string{"daemon-reload"}},
		{Cmd: "systemctl", Args: []string{"enable", "--now", "kubeadm.service"}},
	}

	commands, err := RawIgnitionToProvisioningCommands(cloudData)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(commands).To(HaveLen(len(expectedCmds)))

	for i, cmd := range commands {
		expected := expectedCmds[i]
		g.Expect(cmd.Cmd).To(Equal(expected.Cmd))
		g.Expect(cmd.Args).To(ConsistOf(expected.Args))
	}
}

func TestDecodeFileContents(t *testing.T) {
	g := NewWithT(t)

	in := "data:,foo%20bar"
	out, err := decodeFileContents(in)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(out).To(Equal("foo bar"))
}
