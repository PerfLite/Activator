package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/text/encoding/charmap"
)

type DiskInfo struct {
	Letter      string `json:"letter"`
	TotalGB     string `json:"totalGb"`
	FreeGB      string `json:"freeGb"`
	UsedGB      string `json:"usedGb"`
	PercentUsed int    `json:"percentUsed"`
}

type HardwareSpecs struct {
	CPUName      string     `json:"cpuName"`
	CPUCores     int        `json:"cpuCores"`
	TotalRAM     string     `json:"totalRam"`
	AvailRAM     string     `json:"availRam"`
	UsedRAM      string     `json:"usedRam"`
	RAMPercent   int        `json:"ramPercent"`
	GPUs         []string   `json:"gpus"`
	Motherboard  string     `json:"motherboard"`
	BIOSInfo     string     `json:"biosInfo"`
	Disks        []DiskInfo `json:"disks"`
	OSName       string     `json:"osName"`
	OSVersion    string     `json:"osVersion"`
	OSBuild      string     `json:"osBuild"`
	Arch         string     `json:"arch"`
	ComputerName string     `json:"computerName"`
	UserName     string     `json:"userName"`
	Uptime       string     `json:"uptime"`
}

type SystemInfo struct {
	OSName         string `json:"osName"`
	DisplayVersion string `json:"displayVersion"`
	BuildNumber    string `json:"buildNumber"`
	EditionID      string `json:"editionId"`
	Arch           string `json:"arch"`
	IsAdmin        bool   `json:"isAdmin"`
	DefaultKey     string `json:"defaultKey"`
	DefaultServer  string `json:"defaultServer"`
}

type CommandResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

type ActivatorService struct {
	app *application.App
}

func NewActivatorService() *ActivatorService {
	return &ActivatorService{}
}

func (s *ActivatorService) SetApp(app *application.App) {
	s.app = app
}

func (s *ActivatorService) emitLog(msg string) {
	if s.app != nil {
		s.app.Event.Emit("cmd-output", msg)
	}
}

func checkIsAdmin() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

func cleanKey(key string) string {
	r := strings.NewReplacer(
		"\"", "",
		"'", "",
		"”", "",
		"“", "",
		"«", "",
		"»", "",
		" ", "",
		"\t", "",
		"\r", "",
		"\n", "",
	)
	return strings.ToUpper(r.Replace(key))
}

func decodeWindowsOutput(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	// Декодируем из CP866 (стандартная русская консоль Windows)
	decoded, err := charmap.CodePage866.NewDecoder().Bytes(b)
	if err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	// Попытка Windows-1251
	decoded, err = charmap.Windows1251.NewDecoder().Bytes(b)
	if err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	return string(b)
}

func (s *ActivatorService) runSlmgr(args ...string) CommandResult {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	slmgrVbs := filepath.Join(systemRoot, "System32", "slmgr.vbs")
	cscriptExe := filepath.Join(systemRoot, "System32", "cscript.exe")

	cmdArgs := append([]string{"//nologo", slmgrVbs}, args...)
	cmdStr := fmt.Sprintf("slmgr %s", strings.Join(args, " "))
	s.emitLog(fmt.Sprintf("▶ %s", cmdStr))

	cmd := exec.Command(cscriptExe, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		errMsg := fmt.Sprintf("Ошибка создания пайпа: %v", err)
		s.emitLog(fmt.Sprintf("✖ %s", errMsg))
		return CommandResult{Success: false, Error: errMsg}
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		errMsg := fmt.Sprintf("Ошибка запуска cscript: %v", err)
		s.emitLog(fmt.Sprintf("✖ %s", errMsg))
		return CommandResult{Success: false, Error: errMsg}
	}

	var sb strings.Builder
	reader := bufio.NewReader(stdoutPipe)
	buf := make([]byte, 1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			text := decodeWindowsOutput(buf[:n])
			sb.WriteString(text)
			s.emitLog(text)
		}
		if readErr != nil {
			break
		}
	}

	waitErr := cmd.Wait()
	res := sb.String()
	if waitErr != nil {
		s.emitLog(fmt.Sprintf("✖ Код завершения: %v", waitErr))
		return CommandResult{Success: false, Output: res, Error: waitErr.Error()}
	}
	s.emitLog("✔ Команда успешно выполнена.")
	return CommandResult{Success: true, Output: res}
}

// GetSystemInfo возвращает данные об ОС и статусе прав администратора
func (s *ActivatorService) GetSystemInfo() SystemInfo {
	info := SystemInfo{
		OSName:         "Windows",
		DisplayVersion: "",
		BuildNumber:    "",
		EditionID:      "",
		Arch:           runtime.GOARCH,
		IsAdmin:        checkIsAdmin(),
		DefaultKey:     "W269N-WFGWX-YVC9B-4J6C9-T83GX", // Из promt.txt
		DefaultServer:  "kms8.msguides.com",
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if val, _, err := k.GetStringValue("ProductName"); err == nil {
			info.OSName = val
		}
		if val, _, err := k.GetStringValue("DisplayVersion"); err == nil {
			info.DisplayVersion = val
		}
		if val, _, err := k.GetStringValue("CurrentBuildNumber"); err == nil {
			info.BuildNumber = val
		}
		if val, _, err := k.GetStringValue("EditionID"); err == nil {
			info.EditionID = val
		}
	}
	return info
}

// RelaunchAsAdmin перезапускает приложение с повышением UAC
func (s *ActivatorService) RelaunchAsAdmin() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))

	var showCmd int32 = windows.SW_SHOWNORMAL
	err = windows.ShellExecute(0, verbPtr, exePtr, nil, cwdPtr, showCmd)
	if err != nil {
		return false
	}
	if s.app != nil {
		s.app.Quit()
	}
	return true
}

// InstallKey выполняет команду из promt.txt: slmgr /ipk <KEY>
func (s *ActivatorService) InstallKey(key string) CommandResult {
	cleaned := cleanKey(key)
	if cleaned == "" {
		errMsg := "Ключ продукта не может быть пустым"
		s.emitLog("✖ " + errMsg)
		return CommandResult{Success: false, Error: errMsg}
	}
	s.emitLog(fmt.Sprintf("Установка ключа продукта: %s", cleaned))
	return s.runSlmgr("/ipk", cleaned)
}

// SetKMSServer задает KMS-сервер: slmgr /skms <SERVER>
func (s *ActivatorService) SetKMSServer(server string) CommandResult {
	srv := strings.TrimSpace(server)
	if srv == "" {
		srv = "kms8.msguides.com"
	}
	s.emitLog(fmt.Sprintf("Настройка адреса службы KMS: %s", srv))
	return s.runSlmgr("/skms", srv)
}

// Activate запускает активацию: slmgr /ato
func (s *ActivatorService) Activate() CommandResult {
	s.emitLog("Запуск сетевой активации Windows...")
	return s.runSlmgr("/ato")
}

// FullActivation выполняет полный цикл активации
func (s *ActivatorService) FullActivation(key string, server string) CommandResult {
	s.emitLog("════════════ НАЧАЛО ПОЛНОЙ АКТИВАЦИИ ════════════")
	
	// Шаг 1: Установка ключа продукта (из promt.txt)
	res1 := s.InstallKey(key)
	if !res1.Success {
		s.emitLog("✖ Не удалось установить ключ. Проверьте права администратора.")
		return res1
	}

	// Шаг 2: Установка KMS-сервера
	res2 := s.SetKMSServer(server)
	if !res2.Success {
		s.emitLog("✖ Не удалось задать KMS-сервер.")
		return res2
	}

	// Шаг 3: Активация
	res3 := s.Activate()
	if !res3.Success {
		s.emitLog("✖ Ошибка при выполнении /ato. Возможно KMS-сервер временно недоступен.")
		return res3
	}

	// Шаг 4: Проверка статуса
	s.emitLog("Проверка срока действия лицензии...")
	s.runSlmgr("/xpr")

	s.emitLog("════════════ АКТИВАЦИЯ УСПЕШНО ЗАВЕРШЕНА ════════════")
	return CommandResult{
		Success: true,
		Output:  "Активация Windows успешно завершена!",
	}
}

// CheckStatus выполняет проверку срока окончания лицензии: slmgr /xpr
func (s *ActivatorService) CheckStatus() CommandResult {
	s.emitLog("Запрос срока действия активации (/xpr)...")
	return s.runSlmgr("/xpr")
}

// CheckDetails выполняет подробный отчет о лицензии: slmgr /dli
func (s *ActivatorService) CheckDetails() CommandResult {
	s.emitLog("Запрос информации о лицензии (/dli)...")
	return s.runSlmgr("/dli")
}

// ResetKey выполняет удаление ключа и очистку из реестра: slmgr /upk + slmgr /cpky
func (s *ActivatorService) ResetKey() CommandResult {
	s.emitLog("Удаление ключа продукта (/upk)...")
	res := s.runSlmgr("/upk")
	s.emitLog("Очистка ключа из реестра (/cpky)...")
	s.runSlmgr("/cpky")
	return res
}

// Rearm выполняет сброс таймера активации: slmgr /rearm
func (s *ActivatorService) Rearm() CommandResult {
	s.emitLog("Сброс состояния активации системы (/rearm)...")
	return s.runSlmgr("/rearm")
}

var (
	kernel32Dll             = windows.NewLazySystemDLL("kernel32.dll")
	globalMemoryStatusExProc = kernel32Dll.NewProc("GlobalMemoryStatusEx")
	getTickCount64Proc       = kernel32Dll.NewProc("GetTickCount64")
	getLogicalDrivesProc     = kernel32Dll.NewProc("GetLogicalDrives")
	getDriveTypeWProc        = kernel32Dll.NewProc("GetDriveTypeW")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func getMemoryInfo() (totalGB, availGB, usedGB string, percent int) {
	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	ret, _, _ := globalMemoryStatusExProc.Call(uintptr(unsafe.Pointer(&mem)))
	if ret == 0 {
		return "N/A", "N/A", "N/A", 0
	}
	tot := float64(mem.TotalPhys) / (1024 * 1024 * 1024)
	avail := float64(mem.AvailPhys) / (1024 * 1024 * 1024)
	used := tot - avail
	p := int(mem.MemoryLoad)
	return fmt.Sprintf("%.1f GB", tot), fmt.Sprintf("%.1f GB", avail), fmt.Sprintf("%.1f GB", used), p
}

func getDrivesInfo() []DiskInfo {
	var disks []DiskInfo
	mask, _, _ := getLogicalDrivesProc.Call()
	for i := 0; i < 26; i++ {
		if (mask & (1 << i)) != 0 {
			letter := string(rune('A'+i)) + ":\\"
			lpRootPathName := windows.StringToUTF16Ptr(letter)
			dt, _, _ := getDriveTypeWProc.Call(uintptr(unsafe.Pointer(lpRootPathName)))
			// DRIVE_FIXED = 3
			if dt == 3 {
				var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
				err := windows.GetDiskFreeSpaceEx(
					lpRootPathName,
					&freeBytesAvailable,
					&totalNumberOfBytes,
					&totalNumberOfFreeBytes,
				)
				if err == nil && totalNumberOfBytes > 0 {
					totGB := float64(totalNumberOfBytes) / (1024 * 1024 * 1024)
					freeGB := float64(totalNumberOfFreeBytes) / (1024 * 1024 * 1024)
					usedGB := totGB - freeGB
					percent := int((usedGB / totGB) * 100)
					disks = append(disks, DiskInfo{
						Letter:      string(rune('A'+i)) + ":",
						TotalGB:     fmt.Sprintf("%.1f GB", totGB),
						FreeGB:      fmt.Sprintf("%.1f GB", freeGB),
						UsedGB:      fmt.Sprintf("%.1f GB", usedGB),
						PercentUsed: percent,
					})
				}
			}
		}
	}
	return disks
}

func getSystemUptime() string {
	r, _, _ := getTickCount64Proc.Call()
	if r == 0 {
		return "N/A"
	}
	d := time.Duration(r) * time.Millisecond
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	days := hours / 24
	hours = hours % 24
	if days > 0 {
		return fmt.Sprintf("%d дн %d ч %d мин", days, hours, mins)
	}
	return fmt.Sprintf("%d ч %d мин", hours, mins)
}

func getGPUs() []string {
	var gpus []string
	keyPath := `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return []string{"Не удалось определить"}
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err == nil {
		for _, sub := range subkeys {
			if len(sub) == 4 {
				sk, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath+`\`+sub, registry.QUERY_VALUE)
				if err == nil {
					desc, _, err := sk.GetStringValue("DriverDesc")
					if err == nil && desc != "" {
						gpus = append(gpus, desc)
					}
					sk.Close()
				}
			}
		}
	}
	if len(gpus) == 0 {
		gpus = append(gpus, "Стандартный видеоадаптер")
	}
	return gpus
}

func getCPUInfo() (string, int) {
	cpuName := "Неизвестный процессор"
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if val, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			cpuName = strings.TrimSpace(val)
		}
	}
	return cpuName, runtime.NumCPU()
}

func getMotherboardInfo() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return "Не определено"
	}
	defer k.Close()
	mfg, _, _ := k.GetStringValue("BaseBoardManufacturer")
	prod, _, _ := k.GetStringValue("BaseBoardProduct")
	res := strings.TrimSpace(mfg + " " + prod)
	if res == "" {
		sysMfg, _, _ := k.GetStringValue("SystemManufacturer")
		sysProd, _, _ := k.GetStringValue("SystemProductName")
		res = strings.TrimSpace(sysMfg + " " + sysProd)
	}
	if res == "" {
		return "Не определено"
	}
	return res
}

func getBIOSInfo() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	ver, _, _ := k.GetStringValue("BIOSVersion")
	date, _, _ := k.GetStringValue("BIOSReleaseDate")
	return strings.TrimSpace(ver + " (" + date + ")")
}

// GetHardwareSpecs возвращает подробные аппаратные характеристики ПК
func (s *ActivatorService) GetHardwareSpecs() HardwareSpecs {
	cpuName, cpuCores := getCPUInfo()
	totalRAM, availRAM, usedRAM, ramPercent := getMemoryInfo()
	gpus := getGPUs()
	mobo := getMotherboardInfo()
	bios := getBIOSInfo()
	disks := getDrivesInfo()
	uptime := getSystemUptime()

	hostname, _ := os.Hostname()
	username := os.Getenv("USERNAME")

	sysInfo := s.GetSystemInfo()

	return HardwareSpecs{
		CPUName:      cpuName,
		CPUCores:     cpuCores,
		TotalRAM:     totalRAM,
		AvailRAM:     availRAM,
		UsedRAM:      usedRAM,
		RAMPercent:   ramPercent,
		GPUs:         gpus,
		Motherboard:  mobo,
		BIOSInfo:     bios,
		Disks:        disks,
		OSName:       sysInfo.OSName,
		OSVersion:    sysInfo.DisplayVersion,
		OSBuild:      sysInfo.BuildNumber,
		Arch:         sysInfo.Arch,
		ComputerName: hostname,
		UserName:     username,
		Uptime:       uptime,
	}
}

// PrintSpecsToConsole выводит характеристики ПК прямо в консоль приложения
func (s *ActivatorService) PrintSpecsToConsole() CommandResult {
	specs := s.GetHardwareSpecs()
	s.emitLog("════════════ ХАРАКТЕРИСТИКИ КОМПЬЮТЕРА ════════════")
	s.emitLog(fmt.Sprintf("💻 Компьютер: %s (Пользователь: %s)", specs.ComputerName, specs.UserName))
	s.emitLog(fmt.Sprintf("🪟 ОС: %s %s [Сборка %s, %s]", specs.OSName, specs.OSVersion, specs.OSBuild, specs.Arch))
	s.emitLog(fmt.Sprintf("⏱ Время работы: %s", specs.Uptime))
	s.emitLog(fmt.Sprintf("⚡ Процессор: %s (%d потоков)", specs.CPUName, specs.CPUCores))
	s.emitLog(fmt.Sprintf("🧠 Память RAM: Всего %s, Занято %s (%d%%), Свободно %s", specs.TotalRAM, specs.UsedRAM, specs.RAMPercent, specs.AvailRAM))
	if len(specs.GPUs) > 0 {
		s.emitLog(fmt.Sprintf("🎮 Видеокарта: %s", strings.Join(specs.GPUs, ", ")))
	}
	if specs.Motherboard != "" && specs.Motherboard != "Не определено" {
		s.emitLog(fmt.Sprintf("🔌 Мат. плата: %s (BIOS: %s)", specs.Motherboard, specs.BIOSInfo))
	}
	if len(specs.Disks) > 0 {
		var diskStrs []string
		for _, d := range specs.Disks {
			diskStrs = append(diskStrs, fmt.Sprintf("%s %s (свободно %s)", d.Letter, d.TotalGB, d.FreeGB))
		}
		s.emitLog(fmt.Sprintf("💾 Накопители: %s", strings.Join(diskStrs, " | ")))
	}
	s.emitLog("════════════════════════════════════════════════════")
	return CommandResult{Success: true, Output: "Характеристики выведены в консоль"}
}

