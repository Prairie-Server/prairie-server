module github.com/prairie-server/prairie-server

go 1.27.4

require (
	filippo.io/age v1.3.2
	github.com/DataDog/go-sqllexer v0.2.6
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/abadojack/whatlanggo v1.0.1
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/andybalholm/brotli v1.3.0
	github.com/aws/aws-sdk-go-v2 v1.48.0
	github.com/aws/aws-sdk-go-v2/credentials v1.21.1
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.23.25
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.10
	github.com/aws/smithy-go v1.28.9
	github.com/danielgtaylor/huma/v2 v2.38.0
	github.com/getkin/kin-openapi v0.153.0
	github.com/go-chi/chi/v5 v5.5.0
	github.com/go-chi/cors v1.2.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/certificate-transparency-go v1.4.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/h2non/bimg v1.1.9
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.8.0
	github.com/jackc/pgx/v5 v5.13.0
	github.com/joho/godotenv v1.5.1
	github.com/klauspost/compress v1.20.4
	github.com/miekg/dns v1.1.73
	github.com/mmcdole/gofeed v1.5.0
	github.com/oasdiff/oasdiff v1.38.0
	github.com/oklog/ulid/v2 v2.1.2
	github.com/open-policy-agent/opa v1.22.4
	github.com/pgvector/pgvector-go v0.4.1
	github.com/pgvector/pgvector-go/pgx v0.4.1
	github.com/prairie-server/prairie-plugin-sdk v0.12.1-0.20261116221220-168efd6f9fcd
	github.com/pressly/goose/v3 v3.30.1
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/client_model v0.6.3
	github.com/prometheus/procfs v0.23.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.23
	github.com/redis/go-redis/v9 v9.23.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	github.com/secure-systems-lab/go-securesystemslib v0.11.1
	github.com/sigstore/protobuf-specs v0.5.2
	github.com/sigstore/sigstore v1.11.3
	github.com/sigstore/sigstore-go v1.4.0
	github.com/sony/sonyflake/v2 v2.2.0
	github.com/tetratelabs/wazero v1.12.0
	github.com/theupdateframework/go-tuf/v2 v2.4.2
	github.com/wneessen/go-mail v0.8.2
	github.com/zishang520/socket.io/v2 v2.5.0
	go.n16f.net/thumbhash v1.1.0
	go.opentelemetry.io/contrib/bridges/otelslog v0.21.0
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.23.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.23.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.47.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.47.0
	go.opentelemetry.io/otel/log v1.47.0
	go.opentelemetry.io/otel/sdk v1.47.0
	go.opentelemetry.io/otel/sdk/log v1.47.0
	go.opentelemetry.io/otel/trace v1.47.0
	golang.org/x/crypto v0.59.0
	golang.org/x/image v0.48.0
	golang.org/x/mod v0.43.0
	golang.org/x/net v0.62.0
	golang.org/x/sync v0.25.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	golang.org/x/time v0.16.0
	golang.org/x/tools v0.52.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261116203732-fd2c7689a808
	google.golang.org/grpc v1.85.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.62.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	filippo.io/hpke v0.4.0 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/agnivade/levenshtein v1.2.2 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.12 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyberphone/json-canonicalization v0.0.0-20241213102144-19d51d7fe467 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/digitorus/pkcs7 v0.0.0-20260821105541-05f79448fa77 // indirect
	github.com/digitorus/timestamp v0.0.0-20231217203849-220c5c2851b7 // indirect
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v1.0.0 // indirect
	github.com/go-openapi/errors v0.22.9 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/go-openapi/jsonreference v1.0.2 // indirect
	github.com/go-openapi/loads v0.25.3 // indirect
	github.com/go-openapi/runtime v0.33.3 // indirect
	github.com/go-openapi/runtime/server-middleware v0.33.3 // indirect
	github.com/go-openapi/spec v1.0.1 // indirect
	github.com/go-openapi/strfmt v0.27.4 // indirect
	github.com/go-openapi/swag/conv v0.29.2 // indirect
	github.com/go-openapi/swag/fileutils v0.29.2 // indirect
	github.com/go-openapi/swag/jsonutils v0.29.2 // indirect
	github.com/go-openapi/swag/loading v0.29.2 // indirect
	github.com/go-openapi/swag/mangling v0.29.2 // indirect
	github.com/go-openapi/swag/pools v0.29.2 // indirect
	github.com/go-openapi/swag/stringutils v0.29.2 // indirect
	github.com/go-openapi/swag/typeutils v0.29.2 // indirect
	github.com/go-openapi/swag/yamlutils v0.29.2 // indirect
	github.com/go-openapi/validate v1.0.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v1.1.0 // indirect
	github.com/goccy/go-json v0.11.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-containerregistry v0.22.1 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.32.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/in-toto/attestation v1.2.0 // indirect
	github.com/in-toto/in-toto-golang v0.11.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/jedisct1/go-minisign v0.0.0-20211028175153-1c139d1cc84b // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/dsig v1.4.0 // indirect
	github.com/lestrrat-go/dsig-secp256k1 v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc/v3 v3.0.6 // indirect
	github.com/lestrrat-go/jwx/v3 v3.4.0 // indirect
	github.com/lestrrat-go/option/v2 v2.0.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mfridman/interpolate v0.2.0 // indirect
	github.com/mmcdole/goxpp/v2 v2.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.60.0 // indirect
	github.com/quic-go/webtransport-go v0.11.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sassoftware/relic/v8 v8.2.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	github.com/shibumi/go-pathspec v1.3.0 // indirect
	github.com/sigstore/rekor v1.5.5 // indirect
	github.com/sigstore/rekor-tiles/v2 v2.4.0 // indirect
	github.com/sigstore/timestamp-authority/v2 v2.2.0 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.1.0 // indirect
	github.com/theupdateframework/go-tuf v0.7.0 // indirect
	github.com/tidwall/gjson v1.20.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.2 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/transparency-dev/formats v0.1.1 // indirect
	github.com/transparency-dev/merkle v0.0.2 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/vektah/gqlparser/v2 v2.5.64 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wI2L/jsondiff v0.7.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	github.com/yargevad/filepathx v1.0.0 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zishang520/engine.io-go-parser v1.3.2 // indirect
	github.com/zishang520/engine.io/v2 v2.5.0 // indirect
	github.com/zishang520/socket.io-go-parser/v2 v2.5.0 // indirect
	github.com/zishang520/webtransport-go v0.9.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.1 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/term v0.48.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261110000943-09e21b237f2c // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	modernc.org/libc v1.80.2 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/zishang520/webtransport-go => ./internal/compat/zishang520-webtransport-go
