module github.com/seabird-chat/seabird-discord-backend

go 1.23

toolchain go1.23.2

require (
	github.com/bwmarrin/discordgo v0.27.2-0.20240202235938-7f80bc797881
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-isatty v0.0.20
	github.com/rs/zerolog v1.32.0
	github.com/seabird-chat/seabird-go v0.4.1-0.20240221063203-d8e69692c30b
	github.com/stretchr/testify v1.8.4
	github.com/yuin/goldmark v1.7.8
	golang.org/x/sync v0.6.0
	google.golang.org/protobuf v1.32.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.19.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240221002015-b0ce06bbee7c // indirect
	google.golang.org/grpc v1.61.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/seabird-chat/seabird-go => ../seabird-go

replace github.com/yuin/goldmark => github.com/belak-forks/goldmark v0.0.0-20250104065338-f2faabf722aa
