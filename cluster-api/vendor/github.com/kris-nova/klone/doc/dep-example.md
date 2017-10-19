# Example with [dep](https://github.com/golang/dep)

### Download `klone`

##### Go

```bash
go get -u github.com/kris-nova/klone
```

##### Bash

Where `$ARCH` can be `linux`, `darwin`, `freebsd or `windows`.

```bash
wget https://github.com/kris-nova/klone/releases/download/v1.2.0/$ARCH-amd64
chmod +x $ARCH-amd64
mv $ARCH-amd64 /usr/local/bin/klone
```

### Use `klone` to fork the repository and build a container

```
klone golang/dep -c golang:1.8.3
```

### Make a commit inside the container

The container by default will inherit your SSH, git, and klone configuration, as well as run a shell (`/bin/bash`).

```
cd /go/src/github.com/golang/dep
git checkout -b test-branch
touch hooray
git add hooray
git commit -m "testing klone"
git push origin test-branch
```

### Exit to your host

```bash
exit
```


