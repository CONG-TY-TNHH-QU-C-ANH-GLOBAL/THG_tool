package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/term"
)

// BaseAPI can be overridden via THG_SERVER_URL env var for local/staging use.
var BaseAPI = func() string {
	if v := os.Getenv("THG_SERVER_URL"); v != "" {
		return v
	}
	return "https://sale.thgfulfill.com"
}()

func main() {
	// Simple CLI UI
	fmt.Println("==================================================")
	fmt.Println("       THG FULFILL - CHROME LOGIN AGENT")
	fmt.Println("==================================================")
	fmt.Println("Công cụ này giúp liên kết tài khoản Facebook an toàn")
	fmt.Println("từ máy tính của bạn trực tiếp lên máy chủ THG.")
	fmt.Println()

	// 1. Đăng nhập hệ thống THG (sale.thgfulfill.com)
	var email string
	fmt.Print(">> Nhập Email tài khoản quản lý THG: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)

	fmt.Print(">> Nhập Mật khẩu quản lý THG: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("\n[Lỗi] Không thể đọc mật khẩu.")
		os.Exit(1)
	}
	password := strings.TrimSpace(string(pwBytes))
	fmt.Println("\n\n⏳ Đang xác thực với máy chủ THG...")

	// Gửi Request Login tới VPS
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	resp, err := http.Post(BaseAPI+"/api/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		fmt.Println("❌ Không thể kết nối tới server sale.thgfulfill.com. Vui lòng kiểm tra mạng.")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Sai email hoặc mật khẩu (Status: %d). Chi tiết: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var loginResp struct {
		AccessToken string `json:"access_token"`
		User        struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		fmt.Println("❌ Lỗi dữ liệu trả về từ server.")
		os.Exit(1)
	}

	fmt.Printf("✅ Nhận diện thành công: Xin chào %s!\n\n", loginResp.User.Name)

	for {
		var fbName string
		fmt.Print("\n>> Đặt tên cho tài khoản Facebook này (VD: Nick Chính Sale 1): ")
		fmt.Scanln(&fbName)
		fbName = strings.TrimSpace(fbName)
		if fbName == "" {
			fbName = "Tài khoản Sale"
		}

		fmt.Println("\n🚀 Sắp mở trình duyệt Chrome...")
		fmt.Println("💡 Hướng dẫn: Cửa sổ Chrome sẽ tự động bật lên. Hãy đăng nhập Facebook bình thường trên đó.")
		fmt.Println("Khi bạn đăng nhập Facebook THÀNH CÔNG, chương trình này sẽ tự lấy cookie và đóng Chrome!")
		fmt.Println("Vui lòng đợi vài giây...")
		time.Sleep(2 * time.Second)

		// 2. Mở Chrome hiển thị cho người dùng tự nhập (không dùng UserDataDir để tránh đụng độ extension cá nhân)
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", false), // BẢT BUỘC FALSE ĐỂ HIỂN THỊ CHROME!
			chromedp.Flag("disable-gpu", false),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("no-first-run", true),
			// Tắt popup mặc định
			chromedp.Flag("enable-automation", false),
		)

		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

		ctx, cancel := chromedp.NewContext(allocCtx)

		// 3. Điều khiển Chrome tới trang đăng nhập Facebook
		err = chromedp.Run(ctx,
			chromedp.Navigate("https://www.facebook.com"),
		)
		if err != nil {
			fmt.Printf("❌ Lỗi không mở được Chrome. Bạn có đang cài đặt Chrome trên máy này không? Lỗi: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("🌍 Đã mở Chrome. Xin mời đăng nhập Facebook. Đang theo dõi trạng thái...")

		// 4. Liên tục kiểm tra cookie c_user
		var fbUserID string
		var finalCookies []*network.Cookie

		ticker := time.NewTicker(2 * time.Second)

		// Hạn tối đa 5 phút để thao tác đăng nhập
		timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Minute)

	WaitForLogin:
		for {
			select {
			case <-timeoutCtx.Done():
				fmt.Println("❌ Hết thời gian đăng nhập (5 phút).")
				break WaitForLogin
			case <-ticker.C:
				// Đọc cookie
				if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
					cookies, e := network.GetCookies().WithURLs([]string{"https://www.facebook.com"}).Do(c)
					if e != nil {
						return e
					}
					for _, ck := range cookies {
						if ck.Name == "c_user" && ck.Value != "" {
							fbUserID = ck.Value
							finalCookies = cookies
						}
					}
					return nil
				})); err != nil {
					// Nếu Chrome bị người dùng tắt thủ công
					fmt.Println("❌ Bạn đã đóng Chrome trước khi đăng nhập thành công!")
					break WaitForLogin
				}

				if fbUserID != "" {
					fmt.Printf("✅ Nhận diện đăng nhập thành công Facebook ID: %s\n", fbUserID)
					break WaitForLogin
				}
			}
		}

		if fbUserID != "" {
			// 5. Build Cookie JSON and send to server
			fmt.Println("🔐 Đang thu thập và mã hóa cookie đẩy về Server...")

			type exportCookie struct {
				Name     string  `json:"name"`
				Value    string  `json:"value"`
				Domain   string  `json:"domain"`
				Path     string  `json:"path"`
				Expires  float64 `json:"expires,omitempty"`
				HTTPOnly bool    `json:"httpOnly"`
				Secure   bool    `json:"secure"`
			}

			outCookies := make([]exportCookie, 0, len(finalCookies))
			for _, ck := range finalCookies {
				outCookies = append(outCookies, exportCookie{
					Name:     ck.Name,
					Value:    ck.Value,
					Domain:   ck.Domain,
					Path:     ck.Path,
					Expires:  float64(ck.Expires),
					HTTPOnly: bool(ck.HTTPOnly),
					Secure:   bool(ck.Secure),
				})
			}

			cookieJSON, _ := json.Marshal(outCookies)

			createPayload, _ := json.Marshal(map[string]string{
				"platform":     "facebook",
				"name":         fbName, // Tên tài khoản
				"cookies_json": string(cookieJSON),
				"notes":        "Thêm tự động bằng THG Login Agent",
			})

			req, _ := http.NewRequest("POST", BaseAPI+"/api/accounts", bytes.NewBuffer(createPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)

			createResp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Println("❌ Không thể gửi cookie lên Server:", err)
			} else {
				if createResp.StatusCode != 201 {
					b, _ := io.ReadAll(createResp.Body)
					fmt.Printf("❌ Không thể thêm tài khoản (Status: %d). %s\n", createResp.StatusCode, string(b))
				} else {
					fmt.Println("\n🎉 XONG! Bạn đã lưu tài khoản Facebook lên máy chủ thành công.")
					fmt.Println("Tài khoản của bạn đã được gắn cố định cho tên của bạn trên hệ thống tự động hóa THG.")
				}
				createResp.Body.Close()
			}
		}

		timeoutCancel()
		ticker.Stop()
		cancel()
		allocCancel()

		var addAnother string
		fmt.Print("\n❓ Bạn có muốn thêm một tài khoản Facebook khác không? (y/n): ")
		fmt.Scanln(&addAnother)
		if strings.ToLower(strings.TrimSpace(addAnother)) != "y" {
			break
		}
	}

	fmt.Println("\nCảm ơn bạn đã sử dụng THG Login Agent. Nhấn Enter để thoát...")
	var waitExit string
	fmt.Scanln(&waitExit)
}
