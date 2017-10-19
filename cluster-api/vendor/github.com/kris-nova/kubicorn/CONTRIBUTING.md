# Issues

Remember to craft your issues in such a way as to help others who might be facing similar challenges. 
Give your issues meaningful titles, that offer context.
Please try to use complete sentences in your issue.
Everything is okay to submit as an issue, even questions or ideas.

Issue sizes (small, medium, large) represent amount of effort, and not complexity or skill. 

# Pull Requests 

Try to keep pull requests tidy, and be prepared for feedback.
Everyone is welcome to contribute to `kubicorn` but I do keep a high quality of code standard. 
Be ready to face this.
Feel free to open a pull request for anything, about anything.
Everyone is welcome.

### Formatting Go Code

To get your pull request merged, Golang files must be formatted using the `go fmt` tool.
Please use the `gofmt` make target to format all Golang files.

### Linting

Go code must pass [`lint`](https://github.com/golang/lint) checks. You can run lint on all Golang files using the `lint` make target. Currently, lint checks are not enforced by CI tests but it could change in the future.

### Licensing Header

The CI build will fail if non-vendored Golang files are missing the required licensing header.
Please use the `check-headers` make target or run scripts/check-header.sh to validate.

The `update-headers` make target or scripts/headers.sh will add the necessary headers.

### Vendorning

`kubicorn` project uses [`dep`](https://github.com/golang/dep) to manage its dependencies. Before using it, you need to setup it by following the [Setup guide](https://github.com/golang/dep#setup).

To add new dependency, you need to use:
```
dep ensure import-path
```
If you want to update existing dependency, you need to use:
```
dep ensure -update import-path
```

### Deadlines

I will do my best to keep the pull request backlog clean and tidy.
If you open a pull request and it becomes stale (15+ days) I might leave a note asking you to reopen, and close the pull request.
Please know it means that I am trying to keep our code tidy, and that your pull request is ABSOLUTELY still welcome!

# Testing

As the tool continues to mature we will implement a testing harness.
Once the harness is in place, tests will be required for all bug fixes and features.
No exceptions.

### Happy/Sad Tests

We strive for using the concept of the Happy/Sad tests. Happy test must pass only in case when the given request is correct and response for the request is valid. Sad test must pass only in case when the given request is malformed and appropriate error is returned while parsing it.  

You can take a look at the following [example of Happy/Sad tests](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/godoSdk/sdk_test.go).

# Twitter

You can read the [full documentation](docs/twitter.md) for more information, but be aware that every commit message will be forwarded on to twitter via the `kubicornk8s` account.
If you want to contribute, but do **not** want your commit messages forwarded on to twitter, please send a private message to @kris-nova and she will help!
