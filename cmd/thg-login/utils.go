package main

import (
	"fmt"
	"os"
	"strings"
)

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func browserTargetsConsoleHint(resp browserTargetsResponse) string {
	switch strings.TrimSpace(resp.HintCode) {
	case "no_account_in_org":
		return "Workspace chưa có Facebook account nào để Runtime mở. Tạo phiên Facebook trong Browser dashboard rồi pair lại thiết bị."
	case "assigned_account_missing":
		return "Thiết bị đang gắn với một Facebook account đã bị xóa hoặc không còn thuộc workspace. Disconnect thiết bị và tạo mã kết nối mới."
	case "assigned_account_not_started":
		return "Đang chờ backend tạo phiên Browser local cho account đã gắn. Runtime sẽ tự mở Chrome khi target sẵn sàng."
	case "no_local_session_yet":
		return "Connector online nhưng chưa được gắn với Facebook account cụ thể. Tạo mã kết nối riêng từ đúng account trong Browser dashboard."
	case "no_org":
		return "Connector chưa được gắn vào workspace. Pair lại bằng mã mới từ Browser dashboard."
	}
	if strings.TrimSpace(resp.Hint) != "" {
		return strings.TrimSpace(resp.Hint)
	}
	if resp.AssignedAccountID > 0 {
		return "Đang chờ Browser target cho account đã gắn. Nếu trạng thái này kéo dài, tạo lại mã kết nối từ đúng account."
	}
	return "Connector online nhưng chưa có Browser target. Mở Browser dashboard để chọn Facebook account cần chạy."
}

func printDeviceTokenRejected(err error) {
	fmt.Println("[Connector] Device token is no longer accepted by the dashboard.")
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		fmt.Println("[Connector] Server detail:", err)
	}
	fmt.Println("[Connector] This usually means an admin disconnected this device, the workspace assigned a new connector token, or this app is using an old saved config.")
	fmt.Println("[Connector] Open Browser dashboard, create a new pairing code, then run THG Local Runtime with --reset if you want to connect this device again.")
}

func exitWithError(message string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
