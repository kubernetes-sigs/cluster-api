# Figures with PlantUML
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Generating figures](#generating-figures)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

Most of the figures for this proposal are generated with [PlantUML](http://plantuml.com/), an [open-source](https://sourceforge.net/projects/plantuml/) tool that can generate sequence, use case, class, activity, state, object, and other kinds of UML digrams.

PlantUML requires the Java runtime, so we have published a Docker container image that includes all dependencies (to publish your own, use `Dockerfile` in this directory).

## Generating figures

To generate diagrams in this directory, `make figures`.

In general, to generate the figure described in `foo.plantuml`:
```
SRC="foo.plantuml"
docker run \
	--rm \
	--volume ${PWD}:/figures \
	--user $(shell id --user):$(shell id --group) \
	dpf9/plantuml:1.2019.6 \
	-v /figures/${SRC}
```