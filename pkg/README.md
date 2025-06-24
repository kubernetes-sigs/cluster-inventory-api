# Build and run the controler

go build -out controller.bin controller_example.go
./controller.bin -clusterprofile-provider-file ./cp-creds.json
