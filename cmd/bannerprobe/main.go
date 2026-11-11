package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iamxvbaba/td/exchange"
	"github.com/iamxvbaba/td/telegram"
	"github.com/iamxvbaba/td/telegram/dcs"
	"github.com/iamxvbaba/td/telegram/uploader"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/transport"
	"github.com/redis/go-redis/v9"
	"telesrv/internal/loadharness"
)

const (
	probeDir    = "/root/bannerprobedir"
	telesrvEnv  = "/root/gramsrv/.env"
	redisAddr   = "127.0.0.1:16379"
	redisKeyPfx = "phonecode:"
	phone       = "+155509000001"
	apiID       = 83
	apiHash     = "0411a5fd22e42ba80a41c56fa0a2f9ab"
	firstName   = "ProbeBanner"
)

func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RSA key: %w", err)
	}
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		switch block.Type {
		case "RSA PRIVATE KEY":
			if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				return &key.PublicKey, nil
			}
		case "PRIVATE KEY":
			if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				if rk, ok := key.(*rsa.PrivateKey); ok {
					return &rk.PublicKey, nil
				}
			}
		case "RSA PUBLIC KEY":
			if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
				return key, nil
			}
		case "PUBLIC KEY":
			if p, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
				if rk, ok := p.(*rsa.PublicKey); ok {
					return rk, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no usable RSA key found in %s", path)
}

func envValue(name string) string {
	f, err := os.Open(telesrvEnv)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, name+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, name+"="))
		}
	}
	return ""
}

func fetchCodeFromRedis(hash string) (string, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr, Password: envValue("TELESRV_REDIS_PASSWORD"), DB: 0,
	})
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var rec struct {
		Code string `json:"Code"`
	}
	var last error
	for i := 0; i < 30; i++ {
		raw, err := rdb.Get(ctx, redisKeyPfx+hash).Result()
		if err == nil && raw != "" {
			if err := json.Unmarshal([]byte(raw), &rec); err != nil {
				return "", fmt.Errorf("parse code record: %w", err)
			}
			if rec.Code != "" {
				return rec.Code, nil
			}
		} else if err != nil {
			last = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("code record not found in redis (%s): %w", hash, last)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel2 := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel2()

	if err := os.MkdirAll(probeDir, 0o700); err != nil {
		fmt.Println("MKDIR_FAIL", err)
		return
	}
	if err := run(ctx); err != nil {
		fmt.Println("PROBE_FAIL", err)
		return
	}
	fmt.Println("BANNER_PROBE_DONE")
}

func run(ctx context.Context) error {
	host, portText, err := net.SplitHostPort("127.0.0.1:2398")
	if err != nil {
		return err
	}
	port, _ := strconv.Atoi(portText)

	rsaPub, rsaErr := loadRSAPublicKey("/root/gramsrv/data/server_rsa.pem")
	if rsaErr != nil {
		return rsaErr
	}
	keyPath := filepath.Join(probeDir, "session.key")
	var skey [32]byte
	if rawKey, err := os.ReadFile(keyPath); err == nil {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawKey))); err == nil && len(decoded) == 32 {
			copy(skey[:], decoded)
		}
	} else {
		if _, err := rand.Read(skey[:]); err != nil {
			return err
		}
		if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(skey[:])), 0o600); err != nil {
			return err
		}
	}
	stor := &loadharness.EncryptedFileStorage{
		Path: filepath.Join(probeDir, "session.bin"),
	}
	stor.Key = skey

	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		PublicKeys: []exchange.PublicKey{{RSA: rsaPub}},
		DC:     2,
		Device: telegram.DeviceTDesktopWindows(),
		Resolver: dcs.Plain(dcs.PlainOptions{
			Protocol: dcs.Protocol(transport.Intermediate), Obfuscated: false,
		}),
		DCList: dcs.List{Options: []tg.DCOption{{ID: 2, IPAddress: host, Port: port, Static: true}}},
		SessionStorage: stor,
		UpdateHandler: telegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})
	return client.Run(ctx, func(ctx context.Context) error {
		raw := tg.NewClient(client)
		users0, err := raw.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		authed := err == nil && len(users0) > 0
		if authed {
			fmt.Println("ALREADY_AUTHORIZED")
		} else {
			fmt.Println("AUTH_START", phone)
			sent, err := raw.AuthSendCode(ctx, &tg.AuthSendCodeRequest{
				PhoneNumber: phone, APIID: apiID, APIHash: apiHash, Settings: tg.CodeSettings{},
			})
			if err != nil {
				return fmt.Errorf("sendCode: %w", err)
			}
			sentCode, ok := sent.(*tg.AuthSentCode)
			if !ok {
				return fmt.Errorf("sendCode returned %T", sent)
			}
			code, err := fetchCodeFromRedis(sentCode.PhoneCodeHash)
			if err != nil {
				return err
			}
			fmt.Println("CODE_FETCHED", code)
			authz, err := raw.AuthSignIn(ctx, &tg.AuthSignInRequest{
				PhoneNumber: phone, PhoneCodeHash: sentCode.PhoneCodeHash, PhoneCode: code,
			})
			if _, ok := authz.(*tg.AuthAuthorizationSignUpRequired); ok && err == nil {
				fmt.Println("SIGNUP_REQUIRED")
				su, err := raw.AuthSignUp(ctx, &tg.AuthSignUpRequest{
					PhoneNumber: phone, PhoneCodeHash: sentCode.PhoneCodeHash,
					FirstName: firstName, LastName: "",
				})
				if err != nil {
					return fmt.Errorf("signUp: %w", err)
				}
				_ = su
				fmt.Println("SIGNED_UP")
			} else if err != nil {
				return fmt.Errorf("signIn: %w", err)
			} else {
				_ = authz
				fmt.Println("SIGNED_IN")
			}
			users, err := raw.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
			if err != nil {
				return fmt.Errorf("self after auth: %w", err)
			}
			if u, ok := users[0].(*tg.User); ok {
				fmt.Println("SELF", u.ID, u.FirstName)
			}
		}

		got, err := raw.PhotosGetProfileBanner(ctx, &tg.InputUserSelf{})
		if err != nil {
			return fmt.Errorf("get banner: %w", err)
		}
		fmt.Printf("GET_BANNER_OK resp=%T\n", got)

		// Save a real image as banner: upload via uploader, then save.
		up := uploader.NewUploader(raw)
		imgPath := "/root/XiroGramBot/assets/xirogram_id.png"
		inputFile, err2 := up.FromPath(ctx, imgPath)
		if err2 != nil {
			return fmt.Errorf("upload: %w", err2)
		}
		fmt.Println("UPLOADED")
		if _, err := raw.PhotosSaveProfileBanner(ctx, &tg.PhotosSaveProfileBannerRequest{
			File: inputFile,
		}); err != nil {
			return fmt.Errorf("save banner: %w", err)
		}
		fmt.Println("SAVE_BANNER_OK")
		got2, err2 := raw.PhotosGetProfileBanner(ctx, &tg.InputUserSelf{})
		if err2 != nil {
			return fmt.Errorf("get banner after save: %w", err2)
		}
		fmt.Printf("GET_AFTER_SAVE photo=%T\n", got2.Photo)
		if _, remErr := raw.PhotosRemoveProfileBanner(ctx); remErr != nil {
			fmt.Println("REMOVE_BANNER_ERR", remErr)
		} else {
			fmt.Println("REMOVE_BANNER_OK")
		}
		got3, err3 := raw.PhotosGetProfileBanner(ctx, &tg.InputUserSelf{})
		if err3 != nil {
			return fmt.Errorf("get banner after remove: %w", err3)
		}
		fmt.Printf("GET_AFTER_REMOVE resp=%T\n", got3)
		return nil
	})
}
