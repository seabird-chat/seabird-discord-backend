module github.com/seabird-chat/seabird-discord-backend

go 1.23.0

toolchain go1.23.6

require (
	github.com/bwmarrin/discordgo v0.28.1
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-isatty v0.0.20
	github.com/rs/zerolog v1.33.0
	github.com/seabird-chat/seabird-go v0.5.0
	github.com/stretchr/testify v1.8.4
	github.com/yuin/goldmark v1.7.8
	golang.org/x/sync v0.11.0
	google.golang.org/protobuf v1.36.5
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.34.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250219182151-9fdb1cabc7b2 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// This fork is needed because CommonMark allows H4-H6, but Discord doesn't
replace github.com/yuin/goldmark => github.com/belak-forks/goldmark v0.0.0-20250104065338-f2faabf722aa
