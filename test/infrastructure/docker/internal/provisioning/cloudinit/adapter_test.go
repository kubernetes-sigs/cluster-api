/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
)

func TestRealUseCase(t *testing.T) {
	g := NewWithT(t)

	cloudData := []byte(`
#cloud-config

# from 1 files
# part-001

---
write_files:
-   content: 'LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5ekNDQWJPZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1EVXlNVEUwTXprd04xb1hEVEk1TURVeE9ERTBNemt3TjFvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBUHJCCnZBV0U4UHhQTHVXVDI2NER3VHBJTnJ0aUVSek8wWEdIVXdmWFcyUWx0QlFLdjdjMkpmVXVmWkJxWnF6RDRLelYKNkliVUdvT2w0R21mNUlFcDRuYk5GRjN4SGNiYk5SUGNvc0U5ZTZNWEpwS2RkeStKUDFoM0pSdVhKZ0kvVDVuZwpYQ0xxVkRsVm9IN1BQSjFTbDJQTjYxSG9ORitBT3doNHFWQmdWZ1FuYStIR3kyenFxejROOVl3R01vQ2NNd0hMCjB2QlR2cC9DeGZ5RGRsaVdpYUM4WG55RUtiNkN0VWMwdjVpTkM1dGs3REQ2TVZoZXFMWVVCTDNwRGN0NEFRVnkKVXIwbkptMW1RVWloRm52ZVVpdGlSelo2NExWdVRuZThrdnVwZlFyUEFQb0R5VDgweXhwOEJMeDlEaXo5ZENwKwpKdGppbFFMOS90WU40MDNiS1MwQ0F3RUFBYU1tTUNRd0RnWURWUjBQQVFIL0JBUURBZ0trTUJJR0ExVWRFd0VCCi93UUlNQVlCQWY4Q0FRQXdEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBQ091K1ZLbkIrNTVzUWs2MjdweEVzVnUKN0tNUlNGSW5qekZqa3Npc2ppSU9YZWFVS29RSFpaY3NvMzJVNjdwNjFMUGFUWmVERnpLRVlTK2RYRnJmeWFiNQp4Z1VVMmgxQ0ZTWDl6UFJscC9PQUx0KzVLVzc2blN3UGtFbDlPeE12VzBNZ2xTaEFaVW0ySjlFYkZJZEt6TFpYCjJUeXpmVmx4WHVMV2RUb0RqRTJ4Rnh2eUpLejh3ajdpbDlTL29hTmVyeFF3eFBYTm9ldmxtUGlqT0taUDN2L0sKb2F0U2VPcXZVZHMwSXVkRU1CRWF1dXBxS0ZmejVYd3B1aTI2OHdOem9keHlQckJLL3dCSDBxRnh0bndFY2ZZRApQMDVMd1RwUCt4dnF4cU5OeUlGUndGWnc1NGhLSXFWZktWcU5pYi9pWVpxNEp6RFc1cFdyUWEzeG9BMklvR009Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/ca.crt
    permissions: '0640'
-   content: 'LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBK3NHOEJZVHcvRTh1NVpQYnJnUEJPa2cydTJJUkhNN1JjWWRUQjlkYlpDVzBGQXEvCnR6WWw5UzU5a0dwbXJNUGdyTlhvaHRRYWc2WGdhWi9rZ1NuaWRzMFVYZkVkeHRzMUU5eWl3VDE3b3hjbWtwMTMKTDRrL1dIY2xHNWNtQWo5UG1lQmNJdXBVT1ZXZ2ZzODhuVktYWTgzclVlZzBYNEE3Q0hpcFVHQldCQ2RyNGNiTApiT3FyUGczMWpBWXlnSnd6QWN2UzhGTytuOExGL0lOMldKYUpvTHhlZklRcHZvSzFSelMvbUkwTG0yVHNNUG94CldGNm90aFFFdmVrTnkzZ0JCWEpTdlNjbWJXWkJTS0VXZTk1U0sySkhObnJndFc1T2Q3eVMrNmw5Q3M4QStnUEoKUHpUTEdud0V2SDBPTFAxMEtuNG0yT0tWQXYzKzFnM2pUZHNwTFFJREFRQUJBb0lCQUFuR29janBUT2ZaUW0vSwoydWFtMk5LbjNCSmtHVnl4SjNNd25ta1EyVXhIT0FVTUFqdG5UZ1dJQVhjdTNyL2ZoeFBWNXhIU2xSSUsxbnZuCnN1WGlOeVVBaThtNXk3cGo4MmJKMUVLS1hoYVdvWGRYMGp5MU1oWUYxeG1EUkFVVWFNc0w5eXVaVFIxTEhFMjEKVUp5bGlxZG1jTVVwczFrQnk4dGh3T0FVVVdZcCs0WlVLU1ZoOXZQYU9RYnNQYVRFV2dReml6NXJZL0Q3dEFCegpjcUxmbUVGZlNLREJtbGR2V3ZMZ2E4eElNb0xBUUhqdlRqNm0wNFRXam5kcHBhemR3R29rdjZWTHBoY3I4eFh1CjJWVUJCblhVYVM5ZWQwR1BvOEk3T3VGMEozZlR5bmt2TkUrV2pKK2dDajM1MlYwMk9hREIzaTV4YzBXVjNtVUkKNW1YL0pwVUNnWUVBL1pGZnpPeXE1RUlhR3FJdk5Xdmt2NUw1enVvZVA3SFRLNW9kbHlZZFNCazdxbEpVOUJXMgpMWTZRUytGQ2xBYzFaUXhpZWdmazd5dCtRMWYxS1Jsd09qcU9zUTV1TS9ma01oSzVEMDFJOWpSSFQ2MnY0NlVvCjV2T2hBays4dlRvZi9CVVhQNG1BSW1sWWZHYnhDOVBkL3V6YTNOcWQ3TXRWOG1jRzh1NXNrbWNDZ1lFQS9TbDEKTEtNQmhhekh4L1k4dXcwYUtsVzNIbUVmaEUxMVJBQVBtRG4zRnIvQ08xTWtMYVBSelZXREJPM2w1REkzMGw4SAphcEdZZDc4VWVQVzQzb2cwdkh1ZU41SWpIYWxPSWR2V1JGRHFMYkI2OC9NSkRCY1lTTVRWMFZSWnEzb3dWM1U4CjhnaklMUWh4WmQyQTJuYlNiR3poYXFBNHFZUXhIa2M0U3NKNGMwc0NnWUVBOU5QZXVoQnhXSDl5a1BDendGTHkKeFA1MmNSQ2dNRVBVYnk4WkR3M2dDL05CSnN6ajllRFl5OWZ3L3pMNmc4OEtBUTBhTUZWYStJcjRHTEhlcHRaSApCQkh5SUlhY1pWVWVZakt0dUZhWThnKzhJdlRDOVh4TXArSG9Qa0ViTFdIbjdBKzVLTUhzbEUwL0FLNnNZdzBvCk5iSWdDRXFWWFVOZk12UERRK0J0dUZVQ2dZRUFod3ZtaGJrdXhyQTBvbWFvWHQvT1pXYjBHRENYTDJ4aWNiUFcKbmMzT0VVU1p5Q3ZCME5iaXhEWXBmaWVweXVFL0JlbkxldjNQNTVEMnlzL0pubXZxTmVGN3RRa3YwbExPYXlGcQpXMmNPaFBEdnBkS3ZzTk5oRVBCdlh3c3dDbGxVRUZOcC8zTFAxYlg3Uit1eElOamh4eFVONm1NdDFyKzlzL2twCi9qZGZLYUVDZ1lFQTFTRExIN0g2QU5IRWllbCtvS1hLSERmN1dEK1pZcGhmYWVrd1RZZDRTeUpKRFBNSjVBWEYKTXU3Z1lLc0FWbDJFYjdCZ21FL3lJa1RTc3hxdklEV2xsUFFqblZmQ2xYRUdmY3JXQ3VMdVFqcXhRVUZuZStMaQpHenZzYUlORm5lUHRpZWEzenQyQks5RlVTcDNjZUlGdVpBVGNTYk9EcThIT0p6dXRTcmVYYnRzPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/ca.key
    permissions: '0600'
-   content: 'LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5ekNDQWJPZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1EVXlNVEUwTXprd09Wb1hEVEk1TURVeE9ERTBNemt3T1Zvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTTViClRxUUpjZWF4UmFQWlFNMWtvamV2elVFNjZWbFdZdDBhN2VpSzljNXNzemludDBrVTBHN29EWnBQWlBiL0NHMXYKenZLdDRTZFlpeXJhN3B1c2tkdTZqYmZwaU0zaVZRcFcwZ1FWaFV0ekNZY1JuNWxXZFJDTjh0NWVRZzJRN0RHbwpNRE9EYXV6KzlmTytsbXVDUDk4TkRRZDlKaG5TVlBTeWNZSkVSVGNkdC9ZWXBJNkUzUFkySEJUaGdYSDBGVU5MCmxhRjNjMkNRN1lTUW1YU0piSU5ucldDemphdEgrbHljUk4vTS9FajE0aUkyOGMxWE5aYjFMVHFqN053amxTL3oKWmVHMjdpbVRkZHlRSXdkVndaRVptWTRPS1dGaEk2Y3FwYjZQL0ZOZTBTUGZNbXJXMThjek1jKy9LbEJ0a1NZZgo1VWxUSmVFalM5ZSsxUjJBNVFjQ0F3RUFBYU1tTUNRd0RnWURWUjBQQVFIL0JBUURBZ0trTUJJR0ExVWRFd0VCCi93UUlNQVlCQWY4Q0FRQXdEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBSEdzbVpkRTVDc2FyMUdaalVTMFVISUkKdUxhZlVLNXpkSjRnc0RhQ29nN1RZdGF3WUkrOElueER2aUJoM3VIS09tZDN4QUhBTjVFaFF0SkR3SVRCTjVJZgoyOEhFY25tZmV4MVFNeXFza2llYXc1VmQ3NVBsZ3BXdWRDT0EyeUtSWG9HNWd1bHVVdWI2Rkg2ZjV3S01FNnR3CmpVUkxnUFM2R0o0d2ozbGNnVDgrKzFHbUx4Q3pYRm43VVBGYjh5TlBkaXl6Y1Vncmk0V253Y3NHOUR6dkhHaFQKZHZCQ1I1NGNxS2ZYcEFIbWlISXpVL2J0MnJ5NXdHMkFZeDdiV2xLYnpVclhBbFp2Q0d2TkI2QXUzUXMveGUwYQo5dDhUVmVhV3JtT0dPMXR5Q3RrTXhDQzYyVUYrMkZrNUc0V09NRlo5ZlVmRnBabVVZS2NyZEFlTWFTRzhDd0E9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/etcd/ca.crt
    permissions: '0640'
-   content: 'LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBemx0T3BBbHg1ckZGbzlsQXpXU2lONi9OUVRycFdWWmkzUnJ0NklyMXpteXpPS2UzClNSVFFidWdObWs5azl2OEliVy9POHEzaEoxaUxLdHJ1bTZ5UjI3cU50K21JemVKVkNsYlNCQldGUzNNSmh4R2YKbVZaMUVJM3kzbDVDRFpEc01hZ3dNNE5xN1A3MTg3NldhNEkvM3cwTkIzMG1HZEpVOUxKeGdrUkZOeDIzOWhpawpqb1RjOWpZY0ZPR0JjZlFWUTB1Vm9YZHpZSkR0aEpDWmRJbHNnMmV0WUxPTnEwZjZYSnhFMzh6OFNQWGlJamJ4CnpWYzFsdlV0T3FQczNDT1ZML05sNGJidUtaTjEzSkFqQjFYQmtSbVpqZzRwWVdFanB5cWx2by84VTE3Ukk5OHkKYXRiWHh6TXh6NzhxVUcyUkpoL2xTVk1sNFNOTDE3N1ZIWURsQndJREFRQUJBb0lCQVFDWTBibTlkVmtxdE9HVAo0OUkveVdUd3hIckc4ZS9adjBYYjVKT0hnVkZrRzgvbUJ4NlBPcURaWVhTaGNHYWZIR09MV0IvMFRKelBYSjFECmtYcmZRcitKNysvLzRTejArOFpxcjcwOFZRdXZ3bk90MlhsT1AxN1djYWtJME5rdDNzTnNTdGZYYmwyRFFaVzMKZXM4K3N1akdNSTRUbTdUWnJwQkgzdFo4MkQ1Qi82V2tVTjV2blFOK1RRcHVmaEw0Ynh0TFRmZE1paTBtWUhQeQp3Y0FnNWVIZ2VvcXQwV1dMV2lGaWw2Q01GZkNRVkhTd3dUTVE0bTFkVytrWVBneFZZRGhUQTk0WStLditHMlppCm1Bb3ZuaEtvWEJleW9JcWU2NXdCdktHQUVDUDNvaHlWc0t1dlNJNGVPYUpzRCtuUC9BOU94MG9GaWJtRktRY2kKdXpoT1ZsckJBb0dCQU43bndpL3NBK0locStTcWFjNjJKaXU2MytFb05IVUhVSlFmRGZQUWZiZUY1Zy9zaDNqVQpJajBSS255OHY1cjJjL0ZJdWZkSUkwMGN1a1NPMGFmQTcxajFjczIvZEFtOERmRTBhMzhCWFp3M1l2YndXTVRSCks5MFkyNENlbkJITWVzaU80N0hmRk1weEtSaDRPZndNdjBqa2wralh4OURCSWxVSXNscEtYQm0zQW9HQkFPeisKa0ZYdkNyekRBUStqRFNjc2xpa2NoQW4raGZaUlZMZmI4dkNSc0p3U0ppcmxvU3ZvWmJJc0EwTGhMZ21TWGlHUApVOEM3Rkt1Wk9sRFMyN3JKYkVPalh6aVFNUW1zWGZIMVRHc0VSUDhERVdsa2o4eVBxenVwY3Z3ck0wTzQ5UlIrClFLcXJaNWhPKzdlRkQxQmhvWVYxYU5jZ1U1V2J0bXZQK2FlRGs4OHhBb0dBSVJPcENDMXdvaFMzQ2phVGZ0NGUKcWV5UUhqdzJGSXVpVkdpTFRIdkt1L245bXExUnFRZHBrVUJEMnNDemVnNUtSQ3F6bGRNNWtjN0tnVFBrUG8xdAp0dml0TVlUUWRrVldtRTFjQ2p1c3BXcStuOEFvbkFRaUN5d09IbmJxMStWTTd3ZnRGODd2cWQ3QzUyT256eFFoCktuTHBhOTdoUXNQMkVVSTZIUlhkdHQ4Q2dZRUE1VG9RSjE0dmw0WlNGMnJSUlF2R0xmdUw1eExOUmdOQ214ZGUKTXJ2b0EvMDE5NVhsdjA5b1ZkNW1SU0VDWTNXMElHZStUWk5tR2RmNlpNU2VqVnRYb1ZCNndINFBRRmo5QVJRUApGdytwSUxNNSt5T3VSdURMY2NpakZDOUF4WWMzWGR3RDlsQVZ3bWJhNTVZR3l1dXp6QjlWQ1ljVjhZWUwrdG5OCmt1NGNZSEVDZ1lFQXdxU2NnNzBLWmJrQjdEVXFGSUt5bkNmNDlid2dwSjdQUVVwNnhzWElrQTlneWtnczk3SUEKbVQ1UkVWL0tNcnZDYlNUNFZNVVdWNE9VMWx0eERZbXp2NzhjRDJZQ3NuWGFiQWFLaXVlQTZ2QmNWWkU1RjRaZgpWeVVaR1dtUnpITDQvbGphUHQrelVCSUgxRVFaQ1h5WTVHOW5veVlxMHUyS0tCM2h6VjJSQkQwPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/etcd/ca.key
    permissions: '0600'
-   content: 'LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5ekNDQWJPZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1EVXlNVEUwTXpreE4xb1hEVEk1TURVeE9ERTBNemt4TjFvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTzhrCklkV01FWlFZQzZOdkdVdE9kWVA0OW5La2pmYUY3MldRaUo0bktQU2xmUE83VWxsZlNxR3IwdlpMS1Q3RlgxK3AKVFlhdTNkN3R5UmFVeEpHWW9HaVhaa2EwdzFUWkZhcWh2NDAvNDJpd2xSSVFaQk0xY0FuRThEdlZHTU80bmQ0VApVSFM3Kzd4V0hSQmdNdDlGd3F1enJ4NE1jUEVqTGsrM2xMenJHdVI5cmx1YmNBVlRDeTV1TEpFVlhieXgzNDViCkc2Ynhnc0VKS2ZoWnBZcUpUM0FuODAycE9TUitaWlEvRVNERHA1aGQ1TWFnZkplZ01Gc045WDB0RkMvaER4SUcKMmdhTnZMTklTU1FTWm0rVmIzSkcxNlYzU2pVSWhJYk1KRTcrVnVNbXYwcHZkY1NLb2YzbldsZ1N4M01uclFNVAp1aXV6dlFEenA3Q0ZrM2hrNDg4Q0F3RUFBYU1tTUNRd0RnWURWUjBQQVFIL0JBUURBZ0trTUJJR0ExVWRFd0VCCi93UUlNQVlCQWY4Q0FRQXdEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBS1VGekNoalBWcnVHaTl6dGJjQS8wVjQKWVJ1VC9OVUxta3ZwaFp0OUxGUWRacTNQamU4QzZTeVlmTEZoZHduSFB4bVF4K0luYXdOazVLeXIxMzEvcHRSdgpuWWRjdDVEOXFEYUpRdWtXVGRaRExCUzZnZWZIYk5NdTlQRkNNTVR5SmI4aGhiK3JCcXMrZmR6cUFuOVh4d0NRCjV2MjIva09ha2VyNWJNYXVzRUMrU2ZQQ2NGSzBSckwrVHR5NG9OdFFZbU5jU0YvcEJ0SWl1dnh3NjhwWkZXaVUKNVk3Yi9yQmpWaEk2Sm9TellieE03bzk4eU5weWdBemw3UlNNWUtpL0ZWb09UaGZEUWkxTWdxZmlWeVFjM3hRQgpoUFJFUkpYMjhOa294SXcvRW9FaGk1akc3V1NSeWZDaHNtZ2FTVUo2bFNLZU5EYjZES0ZBRHBqMHZZWFoyRjg9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/front-proxy-ca.crt
    permissions: '0640'
-   content: 'LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBN3lRaDFZd1JsQmdMbzI4WlMwNTFnL2oyY3FTTjlvWHZaWkNJbmljbzlLVjg4N3RTCldWOUtvYXZTOWtzcFBzVmZYNmxOaHE3ZDN1M0pGcFRFa1ppZ2FKZG1SclREVk5rVnFxRy9qVC9qYUxDVkVoQmsKRXpWd0NjVHdPOVVZdzdpZDNoTlFkTHY3dkZZZEVHQXkzMFhDcTdPdkhneHc4U011VDdlVXZPc2E1SDJ1VzV0dwpCVk1MTG00c2tSVmR2TEhmamxzYnB2R0N3UWtwK0ZtbGlvbFBjQ2Z6VGFrNUpINWxsRDhSSU1Pbm1GM2t4cUI4Cmw2QXdXdzMxZlMwVUwrRVBFZ2JhQm8yOHMwaEpKQkptYjVWdmNrYlhwWGRLTlFpRWhzd2tUdjVXNHlhL1NtOTEKeElxaC9lZGFXQkxIY3lldEF4TzZLN085QVBPbnNJV1RlR1RqendJREFRQUJBb0lCQVFEamRCeHlQcDFobkZWRgpoNkFwVG1EYnUycThzK01LL1cwcnp3TUNXZ0RNWUxLdUtCYzFSanQzOWpQYmFyVzZMSVNBT2ttd3RwWDFPWG12CjdxUGdUNmtTa2g0SFZsc0xVc2NXMm0yVTdaVmd0OE94d01GT3U5N3FpOVJyTkU0dnFtTU5ISlhEMGlDbmk4aHQKRVBLU0Jvb1lRZmxudlRHWFNYejgrWUdSQnBVM2lMUW04Rk5ZK2hoYXV3Tjl5T0pIUUtadFNrS281eTVGOE9jYwpmMXZrL3JqVkp4eXVVemZuNzFSbXY4cThWZmpSbEdqK1VMWDI4YzFlTXpqeHpad0g5MHBTMEljZFJUNndmdU5hCmV3NHFRWWF4ZTNteVpLY0YyZElnNlJhZGNWb0ltRnNrenRHcTZ0REo1eXZUQ3VzbGN1a3VqY2NHajhOeEtya0gKRm56QVBEbEJBb0dCQVBlZGpXbmdUVjNJNkE2cVV2WnFHNThXaTEyL1dFb2dXbncveWwwUTZUdGV3bStkTTZYOAp0b25nR2JWODJmMzNLMUNoU29lTFEzbGRjWHBMSUpLSTF4RDBGcnhxWGRKU0h1M3lqdlNrL0RucEpwUGQvbkJSCktpUzFiTGI5cnA3SCtoanBReDZGcXVqcGFlTnc3eWhYcFRuSlphcFFOYzVzbWtUaG5jejVudmRSQW9HQkFQYzkKSHR4MFExMm5TVXJtMExwdDcyTzBuWlNMR0YzQitlLytFa0F6ekJCbnRTR3hUQ2pweFArWkJlcm50d0x3dWhaLwpBZGk2Zk9ENnpMbDM2K2dRYUoxWFljTW05VWI3c0piUDExN2d1RFJFUG9rNlFDRUExZGNZakhjZmE3cjlTbnZMClhJL3A3dEFkNlZpSkdKc2pnRnBkUWh3SFpEc2NyRllrcnlxYUs2RWZBb0dCQUlNMElvaGxaOWszNlc1TDVlWFgKMTRiMmhTWkppMWpMeCtacVRxbjltZmZ4Z0Fsd1BMdkpLbGZvUFBjampzYTVQMlJiOG9mYnpRYno4bnNnYjhQMQphaS83aGtpVCs5N0QwTXU0YVBOTXNMRm16eUF1MHZGa3NIWC9BL242ZFpxQTBBNS9HeWVESUVxRjA2dkdYWkw4Cnpmbk9zMllKVmxsb3hsMlZSdTRqbm8zaEFvR0FLRHZaRHRVWXRWL296SGlkVlFsWTRLZmUwUEtGeDVRdWdVQ2UKWmJaSUtnOUdhYkk1aTVyblJSVDQ0bzVNdVB6RnU2MTFkbmg2by80TVhNNUlKSjZ1OTVQbHcrVk9HdndRYzZwbApDUHFXMzJLUHJyTTlCbUhsYXJpQyswdXdzMkJPdzdDSFlxQVdEZVlnT0JrdldPZkJGbk9BczZEOFRhWlA0VURkCkJKak1LczhDZ1lFQXJCSFdOaVRBVSs3anl4dVJOc25qQy9rQU9rcmo3SFJGK2VBMktYQXRiOVAwMFNwcVpPYk0KZmI2QnRlMzdUSkgwbVcwVkp2aDBiTHdyaWxISVl2YmJHbVNVUm9Ka1d3ZkJLVlVuditnaTF5NHRjVjBrSit2SwpsS3ZsK1lYUmhRb0czMEdRSnRoa3RnUjFHcm5zVFBDM2xuUlV2OEJKR3gvVjNyOER1MDE2NEpFPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/front-proxy-ca.key
    permissions: '0600'
-   content: 'LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUE0WE9sOUpQZFhCSnUrWjY0VVZrZApER0tsTnErL1BiREQveG03YmgxNmhIclBZSWRXK1o3YXIyRnljRWJqdDRlZE94Tkh3UkxlZ3hWVFJIRTk1V1JqCkt1VndkWGRvbUM2MlpXZnBpSm1tV1BSbVh5VElyZzUwbUxkYXVIQ3NyKzNnYUxvOXk0c0VWSkZRRDlGK2lLcDEKTHdaSjl1Y3RaczRaUjI3OTRPSHVXa2ZqMDFRLzU0TFVNZFRUYlVjNjJCL25iK1VvOWoxU1lTckhDRHpQeWZJQQpBZmFEeWFZK1lrakpBRHdRYjlqODFLWVgzaFJ3NFNaNEZVb0ZVdVp5Z3g5Y0k5VncyNmlINTJQUFNtdC9FK2oxCnNkM0U0UzJ2ZU5RUDhKTTUvK2V4c0RBT0NxN0xnWjBJWW1FaHNUcFZFMHQvZGhKQ1R1aEtKZnNISUxzUDZwVXQKM3dJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/sa.pub
    permissions: '0640'
-   content: 'LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBNFhPbDlKUGRYQkp1K1o2NFVWa2RER0tsTnErL1BiREQveG03YmgxNmhIclBZSWRXCitaN2FyMkZ5Y0VianQ0ZWRPeE5Id1JMZWd4VlRSSEU5NVdSakt1VndkWGRvbUM2MlpXZnBpSm1tV1BSbVh5VEkKcmc1MG1MZGF1SENzciszZ2FMbzl5NHNFVkpGUUQ5RitpS3AxTHdaSjl1Y3RaczRaUjI3OTRPSHVXa2ZqMDFRLwo1NExVTWRUVGJVYzYyQi9uYitVbzlqMVNZU3JIQ0R6UHlmSUFBZmFEeWFZK1lrakpBRHdRYjlqODFLWVgzaFJ3CjRTWjRGVW9GVXVaeWd4OWNJOVZ3MjZpSDUyUFBTbXQvRStqMXNkM0U0UzJ2ZU5RUDhKTTUvK2V4c0RBT0NxN0wKZ1owSVltRWhzVHBWRTB0L2RoSkNUdWhLSmZzSElMc1A2cFV0M3dJREFRQUJBb0lCQVFESnczRGkyQzNEaDIrdgpqNThPbGp6TDU4QkpsN0VEcVoxT1FKNGZwdHdOa2NiamNWdWlHOHRFSjJaK0dzTVNiWmlGMVBSalV0cTEzekRjCjBLZC9Fbjg1VllwMlpiM0NiQk9wM3Z2OTF0d3JRZFlZRWRoVEJQYk44VkdNUExJZTVEandJTFRLNHdlbUUwSGUKVmpMeVpmSm5laTVaZTN5RFE3RVYzN3Z2Tk9MV0FVLzFzbStRS0JQME1FWnI5OHhWSXlERjJNdTh2cE5ZcVdMTgpYQ3B4WGM4ME9reWJXZE9WckhQZGpUOS93R2lCa3ZkWjBUam5hbVlxQWllVWl6ejlVTW5HdHZrUS92UjR4N21RCjhWNWhnRTZwSHVlckIzUmlGVGd1eEEvR01XQmhKWjNjbUNQamU4aHRNc0pYUXFteHhIaFhUbGZqSE9xV1ZzR3AKbDA3STlxUzVBb0dCQVByNFluSEczdUNEWHBFOFdkRUk2MXVFMElVU25QSHdoWnBQTnpWNG5hZFFiT2RpYTRObgpNV2lFblA5cVArMFlZVEVQVDY1dldPaUpnZXlNbW93QXdmMFZITVNtMjlSc2RmRnNRTnJ2NzF2cEpSdnRPMkluCjFZK3hQR3VyNTFZUTB5dDVFdm4wVUNiVTdsVlp6WDFYcjJ0bEY4SElwNTZIaFluQUZhbjB1OVNWQW9HQkFPWDQKVnZycnZyWkZuVEc5TnkxYVVYYTVFUFZHK3lqck1DMnptZGRRUUJISVk3THgwWmRoR3ZWQjFMU1A4Tkd0UDIvcgpqNGxQbE9QVk95cjM2WGVxd3c3R0JKbHdEM2pIZE5Wa2tWMkpPeVZ3T3pIVldVUVJ3MU9BLzJ1WFFPeUYzamZyCjY3Znc0VjRlcEl1K0NxazZkcFIwSGg0MGpIckZlRlRMNDYvTmljZWpBb0dBSU9nS2VHS2IvSklkQnl3RGxzMzEKbGlWZTllUFA0a1VvTDJodGs3eEI1NXM2L0VmQ1V4Tm52ZzJOVEV3Ukg3UlBvaEFnNFgxR0NnOWxrcStJNEF5RgpZdnF1ci9ZMDRyQnA4b0xBS2pURmpLYVFNQTQxK0JQREE3azRjK0d4VG02Y1VabnBiQTZscDhISmtqVlpKVE1uCkZBeklSYWRhbXdXbjg3elUybGoxZTlVQ2dZRUFwYUxINnpTUENUTjh0QTJIeDJldEV5amFtUDlGK1VQa1VKWnkKY00yQlNBMmdHWXZvblBLNCt2c3VXOXJzNWVpMXIwUG4vMHROZndmZTlPVFl5SE02eU5KQkQ4N1JwZmxySWlPcwpPOFdTenpWVnZWL2dTcEhNc01GUnRzbWJYb0JRL05BMDJDaHIrbUZ4dktEbGh0dnYrcDdqN25lRTB3eVZ6ZVdJCm1lQWRvNmNDZ1lCajNnbngyaU9zQUJKdjY4MllleXhwVXhFeHR0MjlzNmp5RVBIQi9HMTFibU14RnpsM1FJZDIKVDJOcVA2U2l6M1pQeTFueHdNdUE2Y1BRRFFHZmlZeFhoT002Y0NZMXd5bEVGRXBwc081c0twQmhmMGIvQUVWRQo0TzMwckVzdkY0M3VuV24rMTF3RTllb3YzVFNqS3dvUkE4SkxmVTRlU0JWNDY3N21tdFpBVWc9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=

        '
    encoding: base64
    owner: root:root
    path: /etc/kubernetes/pki/sa.key
    permissions: '0600'
-   content: "---\napiServer:\n  certSANs:\n  - '10.0.0.223'\n  - test1-apiserver-329764956.us-west-2.elb.amazonaws.com\n\
        \  extraArgs:\n    cloud-provider: aws\napiVersion: kubeadm.k8s.io/v1beta2\n\
        certificatesDir: \"\"\nclusterName: test1\ncontrolPlaneEndpoint: test1-apiserver-329764956.us-west-2.elb.amazonaws.com:6443\n\
        controllerManager:\n  extraArgs:\n    cloud-provider: aws\ndns:\n  type: \"\
        \"\netcd: {}\nimageRepository: \"\"\nkind: ClusterConfiguration\nkubernetesVersion:\
        \ v1.16.0\nnetworking:\n  dnsDomain: cluster.local\n  podSubnet: 192.168.0.0/16\n\
        \  serviceSubnet: 10.96.0.0/12\nscheduler: {}\n\n---\napiVersion: kubeadm.k8s.io/v1beta2\n\
        kind: InitConfiguration\nlocalAPIEndpoint:\n  advertiseAddress: \"\"\n  bindPort:\
        \ 0\nnodeRegistration:\n  criSocket: unix:///var/run/containerd/containerd.sock\n\
        \  kubeletExtraArgs:\n    cloud-provider: aws\n  name: 'ip-10-0-0-223.us-west-2.compute.internal'\n"
    owner: root:root
    path: /run/kubeadm/kubeadm.yaml
    permissions: '0640'
`)

	expectedCmds := []provisioning.Cmd{
		// ca
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/ca.crt /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/etc/kubernetes/pki/ca.crt"}},
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/ca.key /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0600", "/etc/kubernetes/pki/ca.key"}},
		// etcd/ca
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki/etcd"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/etcd/ca.crt /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/etc/kubernetes/pki/etcd/ca.crt"}},
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki/etcd"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/etcd/ca.key /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0600", "/etc/kubernetes/pki/etcd/ca.key"}},
		// front-proxy-ca
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/front-proxy-ca.crt /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/etc/kubernetes/pki/front-proxy-ca.crt"}},
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/front-proxy-ca.key /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0600", "/etc/kubernetes/pki/front-proxy-ca.key"}},
		// sa
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/sa.pub /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/etc/kubernetes/pki/sa.pub"}},
		{Cmd: "mkdir", Args: []string{"-p", "/etc/kubernetes/pki"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /etc/kubernetes/pki/sa.key /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0600", "/etc/kubernetes/pki/sa.key"}},
		// /run/kubeadm/kubeadm.yaml
		{Cmd: "mkdir", Args: []string{"-p", "/run/kubeadm"}},
		{Cmd: "/bin/sh", Args: []string{"-c", "cat > /run/kubeadm/kubeadm.yaml /dev/stdin"}},
		{Cmd: "chmod", Args: []string{"0640", "/run/kubeadm/kubeadm.yaml"}},
	}

	commands, err := RawCloudInitToProvisioningCommands(cloudData, kind.Mapping{KubernetesVersion: semver.MustParse("1.16.0")})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(commands).To(HaveLen(len(expectedCmds)))

	for i, cmd := range commands {
		expected := expectedCmds[i]
		g.Expect(cmd.Cmd).To(Equal(expected.Cmd))
		g.Expect(cmd.Args).To(ConsistOf(expected.Args))
	}
}
