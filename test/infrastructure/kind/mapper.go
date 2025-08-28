/*
Copyright 2023 The Kubernetes Authors.

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

/*
Package kind provide utilities for CAPD interoperability with kindest/node images.

CAPD doesn't have direct dependencies from Kind, however, it is using kindest/node images.

As a consequence CAPD must stay in sync with Kind with regards to how kindest/node images are
run when creating a new DockerMachine.

kindest/node images are created using a specific Kind version, and thus it is required
to keep into account the mapping between the Kubernetes version for a kindest/node and
the Kind version that created that image, so we can run it properly.

NOTE: This is the same reason why in the Kind release documentation there is a specific list
of kindest/node images - uniquely identified by a sha - to be used with a Kind release.

For sake of simplification, we are grouping a set of Kind versions that runs kindest/node images
in the same way into Kind mode.
*/
package kind

import (
	"fmt"

	"github.com/blang/semver/v4"

	clusterapicontainer "sigs.k8s.io/cluster-api/util/container"
)

const (
	kindestNodeImageName = "kindest/node"
)

// Mode defines a set of Kind versions that are running images in a consistent way.
type Mode string

const (
	// ModeNone defined a Mode to be used when creating the CAPD load balancer, which doesn't
	// use a kindest/node image.
	ModeNone Mode = "None"

	// Mode0_19 is a Mode that identifies kind v0.19 and older; all those versions
	// are running images in a consistent way (or are too old at the time of this implementation).
	Mode0_19 Mode = "kind 0.19"

	// Mode0_20 is a Mode that identifies kind v0.20 and newer; all those versions
	// are running images in a consistent way, that differs from Mode0_19 because
	// of the adoption of CgroupnsMode = "private" when running images.
	Mode0_20 Mode = "kind 0.20"

	latestMode = Mode0_20
)

// Mapping defines a mapping between a Kubernetes version, a Mode, and a pre-built Image sha.
type Mapping struct {
	KubernetesVersion semver.Version
	Mode              Mode
	Image             string
}

// preBuiltMappings contains the list of kind images pre-built for a given Kind version.
// IMPORTANT: new version should be added at the beginning of the list, so in case an image for
// a given Kubernetes version is rebuilt with a newer kind version, we are using the latest image.
var preBuiltMappings = []Mapping{

	// TODO: Add pre-built images for newer Kind versions on top
	// Pre-built images for Kind v0.30.
	{
		KubernetesVersion: semver.MustParse("1.34.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a",
	},
	{
		KubernetesVersion: semver.MustParse("1.33.4"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.33.4@sha256:25a6018e48dfcaee478f4a59af81157a437f15e6e140bf103f85a2e7cd0cbbf2",
	},
	{
		KubernetesVersion: semver.MustParse("1.32.8"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.32.8@sha256:abd489f042d2b644e2d033f5c2d900bc707798d075e8186cb65e3f1367a9d5a1",
	},
	{
		KubernetesVersion: semver.MustParse("1.31.12"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.12@sha256:0f5cc49c5e73c0c2bb6e2df56e7df189240d83cf94edfa30946482eb08ec57d2",
	},
	// Pre-built images for Kind v0.29.
	{
		KubernetesVersion: semver.MustParse("1.33.1"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.33.1@sha256:050072256b9a903bd914c0b2866828150cb229cea0efe5892e2b644d5dd3b34f",
	},
	{
		KubernetesVersion: semver.MustParse("1.32.5"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.32.5@sha256:e3b2327e3a5ab8c76f5ece68936e4cafaa82edf58486b769727ab0b3b97a5b0d",
	},
	{
		KubernetesVersion: semver.MustParse("1.31.9"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.9@sha256:b94a3a6c06198d17f59cca8c6f486236fa05e2fb359cbd75dabbfc348a10b211",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.13"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.13@sha256:397209b3d947d154f6641f2d0ce8d473732bd91c87d9575ade99049aa33cd648",
	},
	// Pre-built images for Kind v0.27.
	{
		KubernetesVersion: semver.MustParse("1.33.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.33.0@sha256:02f73d6ae3f11ad5d543f16736a2cb2a63a300ad60e81dac22099b0b04784a4e",
	},
	{
		KubernetesVersion: semver.MustParse("1.32.2"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.32.2@sha256:f226345927d7e348497136874b6d207e0b32cc52154ad8323129352923a3142f",
	},
	{
		KubernetesVersion: semver.MustParse("1.31.6"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.6@sha256:28b7cbb993dfe093c76641a0c95807637213c9109b761f1d422c2400e22b8e87",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.10"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.10@sha256:4de75d0e82481ea846c0ed1de86328d821c1e6a6a91ac37bf804e5313670e507",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.14"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.14@sha256:8703bd94ee24e51b778d5556ae310c6c0fa67d761fae6379c8e0bb480e6fea29",
	},

	// Pre-built images for Kind v0.26.
	{
		KubernetesVersion: semver.MustParse("1.32.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027",
	},
	{
		KubernetesVersion: semver.MustParse("1.31.4"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.4@sha256:2cb39f7295fe7eafee0842b1052a599a4fb0f8bcf3f83d96c7f4864c357c6c30",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.8"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.8@sha256:17cd608b3971338d9180b00776cb766c50d0a0b6b904ab4ff52fd3fc5c6369bf",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.12"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.12@sha256:62c0672ba99a4afd7396512848d6fc382906b8f33349ae68fb1dbfe549f70dec",
	},

	// Pre-built images for Kind v0.25.
	{
		KubernetesVersion: semver.MustParse("1.32.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.32.0@sha256:2458b423d635d7b01637cac2d6de7e1c1dca1148a2ba2e90975e214ca849e7cb",
	},
	{
		KubernetesVersion: semver.MustParse("1.31.2"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.2@sha256:18fbefc20a7113353c7b75b5c869d7145a6abd6269154825872dc59c1329912e",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.6"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.6@sha256:b6d08db72079ba5ae1f4a88a09025c0a904af3b52387643c285442afb05ab994",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.10"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.10@sha256:3b2d8c31753e6c8069d4fc4517264cd20e86fd36220671fb7d0a5855103aa84b",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.15"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.15@sha256:a7c05c7ae043a0b8c818f5a06188bc2c4098f6cb59ca7d1856df00375d839251",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.16@sha256:2d21a61643eafc439905e18705b8186f3296384750a835ad7a005dceb9546d20",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.15"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.15@sha256:c79602a44b4056d7e48dc20f7504350f1e87530fe953428b792def00bc1076dd",
	},

	// Pre-built images for Kind v0.24.
	{
		KubernetesVersion: semver.MustParse("1.31.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.4"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.4@sha256:976ea815844d5fa93be213437e3ff5754cd599b040946b5cca43ca45c2047114",
	},
	{
		KubernetesVersion: semver.MustParse("1.30.3"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.3@sha256:bf91e1ef2f7d92bb7734b2b896b3dddea98f0496b34d96e37dd5d7df331b7e56",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.8"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.8@sha256:d46b7aa29567e93b27f7531d258c372e829d7224b25e3fc6ffdefed12476d3aa",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.7"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.7@sha256:f70ab5d833fca132a100c1f95490be25d76188b053f49a3c0047ff8812360baf",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.13"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.13@sha256:45d319897776e11167e4698f6b14938eb4d52eb381d9e3d7a9086c16c69a8110",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.12"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.12@sha256:fa0e48b1e83bb8688a5724aa7eebffbd6337abd7909ad089a2700bf08c30c6ea",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.17@sha256:3fd82731af34efe19cd54ea5c25e882985bafa2c9baefe14f8deab1737d9fabe",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.15"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.15@sha256:1cc15d7b1edd2126ef051e359bf864f37bbcf1568e61be4d2ed1df7a3e87b354",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.25.16@sha256:6110314339b3b44d10da7d27881849a87e092124afab5956f2e10ecdb463b025",
	},

	// Pre-built images for Kind v0.23.
	{
		KubernetesVersion: semver.MustParse("1.30.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.30.0@sha256:047357ac0cfea04663786a612ba1eaba9702bef25227a794b52890dd8bcd692e",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.4"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.4@sha256:3abb816a5b1061fb15c6e9e60856ec40d56b7b52bcea5f5f1350bc6e2320b6f8",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.9"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.9@sha256:dca54bc6a6079dd34699d53d7d4ffa2e853e46a20cd12d619a09207e35300bd0",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.13"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.13@sha256:17439fa5b32290e3ead39ead1250dca1d822d94a10d26f1981756cd51b24b9d8",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.15"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.15@sha256:84333e26cae1d70361bb7339efb568df1871419f2019c80f9a12b7e2d485fe19",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.25.16@sha256:5da57dfc290ac3599e775e63b8b6c49c0c85d3fec771cd7d55b45fae14b38d3b",
	},

	// Pre-built images for Kind v0.22.
	{
		KubernetesVersion: semver.MustParse("1.29.2"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.2@sha256:51a1434a5397193442f0be2a297b488b6c919ce8a3931be0ce822606ea5ca245",
	},
	{
		KubernetesVersion: semver.MustParse("1.29.1"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.1@sha256:0c06baa545c3bb3fbd4828eb49b8b805f6788e18ce67bff34706ffa91866558b",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.7"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.7@sha256:9bc6c451a289cf96ad0bbaf33d416901de6fd632415b076ab05f5fa7e4f65c58",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.6"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.6@sha256:e9e59d321795595d0eed0de48ef9fbda50388dc8bd4a9b23fb9bd869f370ec7e",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.11"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.11@sha256:681253009e68069b8e01aad36a1e0fa8cf18bb0ab3e5c4069b2e65cafdd70843",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.10"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.10@sha256:e6b2f72f22a4de7b957cd5541e519a8bef3bae7261dd30c6df34cd9bdd3f8476",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.14"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.14@sha256:5d548739ddef37b9318c70cb977f57bf3e5015e4552be4e27e57280a8cbb8e4f",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.13"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.13@sha256:8cb4239d64ff897e0c21ad19fe1d68c3422d4f3c1c1a734b7ab9ccc76c549605",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.25.16@sha256:e8b50f8e06b44bb65a93678a65a26248fae585b3d3c2a669e5ca6c90c69dc519",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.24.17@sha256:bad10f9b98d54586cba05a7eaa1b61c6b90bfc4ee174fdc43a7b75ca75c95e51",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.23.17@sha256:14d0a9a892b943866d7e6be119a06871291c517d279aedb816a4b4bc0ec0a5b3",
	},

	// Pre-built images for Kind v0.21.
	{
		KubernetesVersion: semver.MustParse("1.29.1"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.1@sha256:a0cc28af37cf39b019e2b448c54d1a3f789de32536cb5a5db61a49623e527144",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.6"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.6@sha256:b7e1cf6b2b729f604133c667a6be8aab6f4dde5bb042c1891ae248d9154f665b",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.10"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.10@sha256:3700c811144e24a6c6181065265f69b9bf0b437c45741017182d7c82b908918f",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.13"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.13@sha256:15ae92d507b7d4aec6e8920d358fc63d3b980493db191d7327541fbaaed1f789",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.16"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.25.16@sha256:9d0a62b55d4fe1e262953be8d406689b947668626a357b5f9d0cfbddbebbc727",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.24.17@sha256:ea292d57ec5dd0e2f3f5a2d77efa246ac883c051ff80e887109fabefbd3125c7",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.23.17@sha256:fbb92ac580fce498473762419df27fa8664dbaa1c5a361b5957e123b4035bdcf",
	},

	// Pre-built images for Kind v0.20.
	{
		KubernetesVersion: semver.MustParse("1.29.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.29.0@sha256:eaa1450915475849a73a9227b8f201df25e55e268e5d619312131292e324d570",
	},
	{
		KubernetesVersion: semver.MustParse("1.28.0"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.28.0@sha256:b7a4cad12c197af3ba43202d3efe03246b3f0793f162afb40a33c923952d5b31",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.3"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.27.3@sha256:3966ac761ae0136263ffdb6cfd4db23ef8a83cba8a463690e98317add2c9ba72",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.6"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.26.6@sha256:6e2d8b28a5b601defe327b98bd1c2d1930b49e5d8c512e1895099e4504007adb",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.11"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.25.11@sha256:227fa11ce74ea76a0474eeefb84cb75d8dad1b08638371ecf0e86259b35be0c8",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.15"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.24.15@sha256:7db4f8bea3e14b82d12e044e25e34bd53754b7f2b0e9d56df21774e6f66a70ab",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.23.17@sha256:59c989ff8a517a93127d4a536e7014d28e235fb3529d9fba91b3951d461edfdb",
	},
	{
		KubernetesVersion: semver.MustParse("1.22.17"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.22.17@sha256:f5b2e5698c6c9d6d0adc419c0deae21a425c07d81bbf3b6a6834042f25d4fba2",
	},
	{
		KubernetesVersion: semver.MustParse("1.21.14"),
		Mode:              Mode0_20,
		Image:             "kindest/node:v1.21.14@sha256:8a4e9bb3f415d2bb81629ce33ef9c76ba514c14d707f9797a01e3216376ba093",
	},

	// Pre-built images for Kind v0.19.
	{
		KubernetesVersion: semver.MustParse("1.27.1"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.27.1@sha256:b7d12ed662b873bd8510879c1846e87c7e676a79fefc93e17b2a52989d3ff42b",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.4"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.26.4@sha256:f4c0d87be03d6bea69f5e5dc0adb678bb498a190ee5c38422bf751541cebe92e",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.9"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.25.9@sha256:c08d6c52820aa42e533b70bce0c2901183326d86dcdcbedecc9343681db45161",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.13"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.24.13@sha256:cea86276e698af043af20143f4bf0509e730ec34ed3b7fa790cc0bea091bc5dd",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.17"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.23.17@sha256:f77f8cf0b30430ca4128cc7cfafece0c274a118cd0cdb251049664ace0dee4ff",
	},
	{
		KubernetesVersion: semver.MustParse("1.22.17"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.22.17@sha256:9af784f45a584f6b28bce2af84c494d947a05bd709151466489008f80a9ce9d5",
	},
	{
		KubernetesVersion: semver.MustParse("1.21.14"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.21.14@sha256:220cfafdf6e3915fbce50e13d1655425558cb98872c53f802605aa2fb2d569cf",
	},

	// Pre-built and additional images for Kind v0.18.
	// NOTE: This version predates the introduction of this change, but we are including it to expand
	// the list of supported K8s versions; since they can be started with the same approach used up to kind 0.19,
	// we are considering this version part of the Mode0_19 group.
	{
		KubernetesVersion: semver.MustParse("1.26.3"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.26.3@sha256:61b92f38dff6ccc29969e7aa154d34e38b89443af1a2c14e6cfbd2df6419c66f",
	},
	{
		KubernetesVersion: semver.MustParse("1.25.8"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.25.8@sha256:00d3f5314cc35327706776e95b2f8e504198ce59ac545d0200a89e69fce10b7f",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.12"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.24.12@sha256:1e12918b8bc3d4253bc08f640a231bb0d3b2c5a9b28aa3f2ca1aee93e1e8db16",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.17"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.23.17@sha256:e5fd1d9cd7a9a50939f9c005684df5a6d145e8d695e78463637b79464292e66c",
	},
	{
		KubernetesVersion: semver.MustParse("1.22.17"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.22.17@sha256:c8a828709a53c25cbdc0790c8afe12f25538617c7be879083248981945c38693",
	},
	{
		KubernetesVersion: semver.MustParse("1.21.14"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.21.14@sha256:27ef72ea623ee879a25fe6f9982690a3e370c68286f4356bf643467c552a3888",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.1"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.27.1@sha256:9915f5629ef4d29f35b478e819249e89cfaffcbfeebda4324e5c01d53d937b09",
	},
	{
		KubernetesVersion: semver.MustParse("1.27.0"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.27.0@sha256:c6b22e613523b1af67d4bc8a0c38a4c3ea3a2b8fbc5b367ae36345c9cb844518",
	},

	// Pre-built and additional images for Kind v0.17.
	// NOTE: This version predates the introduction of this change, but we are including it to expand
	// the list of supported K8s versions; since they can be started with the same approach used up to kind 0.19,
	// we are considering this version part of the Mode0_19 group.

	{
		KubernetesVersion: semver.MustParse("1.25.3"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.25.3@sha256:f52781bc0d7a19fb6c405c2af83abfeb311f130707a0e219175677e366cc45d1",
	},
	{
		KubernetesVersion: semver.MustParse("1.24.7"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.24.7@sha256:577c630ce8e509131eab1aea12c022190978dd2f745aac5eb1fe65c0807eb315",
	},
	{
		KubernetesVersion: semver.MustParse("1.23.13"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.23.13@sha256:ef453bb7c79f0e3caba88d2067d4196f427794086a7d0df8df4f019d5e336b61",
	},
	{
		KubernetesVersion: semver.MustParse("1.22.15"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.22.15@sha256:7d9708c4b0873f0fe2e171e2b1b7f45ae89482617778c1c875f1053d4cef2e41",
	},
	{
		KubernetesVersion: semver.MustParse("1.21.14"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.21.14@sha256:9d9eb5fb26b4fbc0c6d95fa8c790414f9750dd583f5d7cee45d92e8c26670aa1",
	},
	{
		KubernetesVersion: semver.MustParse("1.20.15"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.20.15@sha256:a32bf55309294120616886b5338f95dd98a2f7231519c7dedcec32ba29699394",
	},
	{
		KubernetesVersion: semver.MustParse("1.19.16"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.19.16@sha256:476cb3269232888437b61deca013832fee41f9f074f9bed79f57e4280f7c48b7",
	},
	{
		KubernetesVersion: semver.MustParse("1.26.0"),
		Mode:              Mode0_19,
		Image:             "kindest/node:v1.26.0@sha256:691e24bd2417609db7e589e1a479b902d2e209892a10ce375fab60a8407c7352",
	},
}

// GetMapping return the Mapping for a given Kubernetes version/custom image.
// If a custom image is provided, return the corresponding Mapping if defined, otherwise return the mapping
// for target K8sVersion if defined; if there is no exact match for a given Kubernetes version/custom image,
// a best effort mapping is returned.
// NOTE: returning a best guess mapping is a way to try to make things to work when new images are published and
// either CAPD code in this file is not yet updated or users/CI are using older versions of CAPD.
// Even if this can lead to CAPD failing in not obvious ways, we consider this an acceptable trade off given that this
// is a provider to be used for development only and this approach allow us bit more of flexibility in using Kindest/node
// images published after CAPD version is cut (without doing code changes in the list above and/or cherry-picks).
func GetMapping(k8sVersion semver.Version, customImage string) Mapping {
	bestGuess := Mapping{
		KubernetesVersion: k8sVersion,
		Mode:              latestMode,
		Image:             pickFirstNotEmpty(customImage, fallbackImage(k8sVersion)),
	}
	for _, m := range preBuiltMappings {
		// If a custom image is provided and it matches the mapping, return it.
		if m.Image == customImage {
			return m
		}
	}
	for _, m := range preBuiltMappings {
		// If the mapping isn't for the right Major/Minor, ignore it.
		if k8sVersion.Major != m.KubernetesVersion.Major || k8sVersion.Minor != m.KubernetesVersion.Minor {
			continue
		}

		// If the mapping is for the same patch version and no pre version is defined, return it
		if k8sVersion.Patch == m.KubernetesVersion.Patch && len(k8sVersion.Pre) == 0 {
			return Mapping{
				KubernetesVersion: m.KubernetesVersion,
				Mode:              m.Mode,
				Image:             pickFirstNotEmpty(customImage, m.Image),
			}
		}

		// If the mapping is for an older patch version, then the K8s version is newer that any published image we are aware of,
		// so we return out best guess.
		if k8sVersion.Patch > m.KubernetesVersion.Patch {
			return bestGuess
		}

		// Otherwise keep looping in the existing mapping but use the oldest kind mode as a best guess.
		bestGuess.Mode = m.Mode
	}
	return bestGuess
}

// fallbackImage is a machine image for a given K8s version but without a sha.
// NOTE: When using fallback images there is the risk of inconsistencies between
// the kindest/node image that will be returned by the docker registry and the
// mode that will be used to start the image.
func fallbackImage(k8sVersion semver.Version) string {
	versionString := clusterapicontainer.SemverToOCIImageTag(fmt.Sprintf("v%s", k8sVersion.String()))

	return fmt.Sprintf("%s:%s", kindestNodeImageName, versionString)
}

func pickFirstNotEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
