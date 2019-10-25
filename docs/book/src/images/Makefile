FILES := bootstrap-controller.plantuml
FILES += cluster-admission-cluster-controller.plantuml
FILES += cluster-admission-machine-controller.plantuml
FILES +=cluster-admission-machineset-controller.plantuml

svg := $(FILES:.plantuml=.svg)
png := $(FILES:.plantuml=.png)

all: $(svg) $(png)

%.svg: %.plantuml
	plantuml -tsvg $<

%.png: %.plantuml
	plantuml $<
