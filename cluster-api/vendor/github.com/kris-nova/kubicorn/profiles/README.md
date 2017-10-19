# Profiles

A profile is a unique representation of a cluster.
Think of these as handy snippets of Go that represent a cluster.
Ideally you can pass these around from dev to dev, and tweak them to your liking.

It is important to note that a profile comes with no guarantee (by design). 
So it will be up to **you** to make sure your profile works! 
But don't worry, the core of `kubicorn` will help make that easy.

Profiles are `struct{}` literals in Go (by design). So you should always be able to define it just as easy as a configuration file. So no need for YAML abstractions before hand!

### Why are there so many?

Because there are a lot of different combinations of cluster resources a user might want.

### Can I add one to the repo?

Yep. Check out [contributing](https://github.com/kris-nova/kubicorn/blob/master/CONTRIBUTING.md) for more information!

### Can I host my own?

Yep. We have a feature coming soon that will allow you to point to local disk or a remote resource!

### Is it Go or YAML?

Go. Then it turns into YAML, if you are using a state store.
