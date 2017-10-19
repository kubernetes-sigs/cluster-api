# Bootstrap

These are the bootstrap scripts that ship with the default `kubicorn` profiles.

Feel free to add your own, or modify these at any time.

The scripts are effectively what we use as `user data` to init a VM

### I need to template out one of these bootstrap scripts

No you don't. Write bash like a pro.

### I need more data in a bootstrap script what should I do?

If you really can only get it from `kubicorn` and nowhere else, you can use the `Values{}` struct to define custom key/value pairs that will be injected into your script.
This will be a code change, and is intended to be just that.