# State Stores

All clusters are represented via a state, each state needs to be stored somewhere.

This is a directory that defines the interface of how `kubicorn` will interact with a state store, as well as all the implementations we have.

Every new state store will need to be baked into the cobra commands.
