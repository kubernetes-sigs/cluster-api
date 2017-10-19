# Cluster Utilities

This is a package that offers convenience functionality for working with the cluster API object.

### Pronunciation

```
cutil: KUH-TUHL
```

### Cluster Validation

On initialization, we run a variety of validation functions to validate the cluster API object.

To add a new validation, add a new function with a name like `validateThingYouAreValidating` to `initapi/validate.go`.
Your function will need to take as input the cluster object and return an error message if it fails validation.
You will also need happy and sad tests to test your new validation, which can be added in `initapi/validate_test.go`.

To have your validation function run on initialization, you will need to add it to the list of validations found in `initapi/init.go`. 