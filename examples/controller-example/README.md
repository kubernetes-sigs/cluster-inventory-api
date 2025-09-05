# Building and Running the Controller Example

Please execute these steps from the root directory of the repository.

## Prerequisite: Build the Secret Reader plugin

```bash
go build -o ./bin/secretreader-plugin ./cmd/secretreader-plugin
```

## Build the controller example

```bash
go build -o ./examples/controller-example/controller-example.bin ./examples/controller-example
```

## Run

```bash
./examples/controller-example/controller-example.bin -clusterprofile-provider-file ./examples/controller-example/cp-creds.json
```
