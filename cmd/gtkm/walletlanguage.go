package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

// walletLanguageTerms contains the navigation text translated by the
// interactive wallet. Values, addresses, transaction data, and user-entered
// content remain unchanged.
type walletLanguageTerms struct {
	Title            string
	Subtitle         string
	Portfolio        string
	Send             string
	Accounts         string
	Phone            string
	Email            string
	Kings            string
	Refresh          string
	Language         string
	Exit             string
	Select           string
	Closed           string
	LanguageUI       string
	AutoDetect       string
	Detected         string
	Current          string
	Saved            string
	Change           string
	SectionPhone     string
	SectionEmail     string
	SectionKings     string
	SectionAccounts  string
	SectionPortfolio string
	SectionSend      string
}

type walletLanguage struct {
	Code       string
	Name       string
	NativeName string
	Terms      walletLanguageTerms
}

type walletLanguagePreference struct {
	Language string `json:"language"`
}

var walletLanguageFlag = &cli.StringFlag{
	Name:    "language",
	Aliases: []string{"lang"},
	Value:   "auto",
	Usage:   "Wallet and daemon interface language (auto, zh, ru, en, ja, ko, es, pt, fr, de, ar, hi, id, tr, vi, it, nl, pl, uk, th, bn)",
}

var walletActiveLanguage = walletLanguageByCode("en")

// The catalog order is intentional: the most requested languages are first.
// English remains the fallback for a missing translation.
var walletLanguageCatalog = []walletLanguage{
	{Code: "zh", Name: "Chinese", NativeName: "中文", Terms: walletLanguageTerms{Title: "TKM 钱包", Subtitle: "安全本地签名终端", Portfolio: "资产", Send: "发送", Accounts: "账户", Phone: "TKM 电话", Email: "邮箱", Kings: "轮换王", Refresh: "刷新", Language: "语言", Exit: "退出", Select: "请选择", Closed: "钱包已关闭。", LanguageUI: "语言", AutoDetect: "从系统自动检测", Detected: "检测到的语言", Current: "当前语言", Saved: "语言已保存", Change: "更改语言", SectionPhone: "TKM 电话", SectionEmail: "邮箱", SectionKings: "轮换王", SectionAccounts: "本地账户", SectionPortfolio: "资产", SectionSend: "发送资金"}},
	{Code: "ru", Name: "Russian", NativeName: "Русский", Terms: walletLanguageTerms{Title: "TKM КОШЕЛЁК", Subtitle: "Безопасная локальная подпись", Portfolio: "Портфель", Send: "Отправить", Accounts: "Счета", Phone: "TKM Телефон", Email: "Почта", Kings: "Ротация королей", Refresh: "Обновить", Language: "Язык", Exit: "Выход", Select: "Выберите пункт", Closed: "Кошелёк закрыт.", LanguageUI: "Язык", AutoDetect: "Определить по системе", Detected: "Определённый язык", Current: "Текущий язык", Saved: "Язык сохранён", Change: "Изменить язык", SectionPhone: "TKM ТЕЛЕФОН", SectionEmail: "ПОЧТА", SectionKings: "РОТАЦИЯ КОРОЛЕЙ", SectionAccounts: "ЛОКАЛЬНЫЕ СЧЕТА", SectionPortfolio: "ПОРТФЕЛЬ", SectionSend: "ОТПРАВКА СРЕДСТВ"}},
	{Code: "en", Name: "English", NativeName: "English", Terms: walletLanguageTerms{Title: "TKM WALLET", Subtitle: "Secure local signing console", Portfolio: "Portfolio", Send: "Send", Accounts: "Accounts", Phone: "TKM Phone", Email: "Email", Kings: "Kings", Refresh: "Refresh", Language: "Language", Exit: "Exit", Select: "Select an option", Closed: "Wallet closed.", LanguageUI: "Language", AutoDetect: "Auto-detect from system", Detected: "Detected language", Current: "Current language", Saved: "Language saved", Change: "Change language", SectionPhone: "TKM PHONE", SectionEmail: "EMAILVM", SectionKings: "ROTATING KINGS", SectionAccounts: "LOCAL ACCOUNTS", SectionPortfolio: "PORTFOLIO", SectionSend: "SEND FUNDS"}},
	{Code: "ja", Name: "Japanese", NativeName: "日本語", Terms: walletLanguageTerms{Title: "TKM ウォレット", Subtitle: "安全なローカル署名コンソール", Portfolio: "ポートフォリオ", Send: "送信", Accounts: "アカウント", Phone: "TKM 電話", Email: "メール", Kings: "ローテーションキング", Refresh: "更新", Language: "言語", Exit: "終了", Select: "項目を選択", Closed: "ウォレットを閉じました。", LanguageUI: "言語", AutoDetect: "システムから自動検出", Detected: "検出された言語", Current: "現在の言語", Saved: "言語を保存しました", Change: "言語を変更", SectionPhone: "TKM 電話", SectionEmail: "メール", SectionKings: "ローテーションキング", SectionAccounts: "ローカルアカウント", SectionPortfolio: "ポートフォリオ", SectionSend: "資金を送信"}},
	{Code: "ko", Name: "Korean", NativeName: "한국어", Terms: walletLanguageTerms{Title: "TKM 지갑", Subtitle: "안전한 로컬 서명 콘솔", Portfolio: "포트폴리오", Send: "보내기", Accounts: "계정", Phone: "TKM 전화", Email: "이메일", Kings: "순환 킹", Refresh: "새로 고침", Language: "언어", Exit: "종료", Select: "항목을 선택하세요", Closed: "지갑을 닫았습니다.", LanguageUI: "언어", AutoDetect: "시스템에서 자동 감지", Detected: "감지된 언어", Current: "현재 언어", Saved: "언어가 저장되었습니다", Change: "언어 변경", SectionPhone: "TKM 전화", SectionEmail: "이메일", SectionKings: "순환 킹", SectionAccounts: "로컬 계정", SectionPortfolio: "포트폴리오", SectionSend: "자금 보내기"}},
	{Code: "es", Name: "Spanish", NativeName: "Español", Terms: walletLanguageTerms{Portfolio: "Cartera", Send: "Enviar", Accounts: "Cuentas", Phone: "Teléfono TKM", Email: "Correo", Kings: "Reyes rotativos", Refresh: "Actualizar", Language: "Idioma", Exit: "Salir", Select: "Seleccione una opción", Closed: "Cartera cerrada.", LanguageUI: "Idioma", AutoDetect: "Detectar del sistema", Current: "Idioma actual", Saved: "Idioma guardado", Change: "Cambiar idioma", SectionPhone: "TELÉFONO TKM", SectionEmail: "CORREO", SectionKings: "REYES ROTATIVOS", SectionAccounts: "CUENTAS LOCALES", SectionPortfolio: "CARTERA", SectionSend: "ENVIAR FONDOS"}},
	{Code: "pt", Name: "Portuguese", NativeName: "Português", Terms: walletLanguageTerms{Portfolio: "Carteira", Send: "Enviar", Accounts: "Contas", Phone: "Telefone TKM", Email: "E-mail", Kings: "Reis rotativos", Refresh: "Atualizar", Language: "Idioma", Exit: "Sair", Select: "Selecione uma opção", Closed: "Carteira fechada.", LanguageUI: "Idioma", AutoDetect: "Detectar pelo sistema", Current: "Idioma atual", Saved: "Idioma salvo", Change: "Mudar idioma", SectionPhone: "TELEFONE TKM", SectionEmail: "E-MAIL", SectionKings: "REIS ROTATIVOS", SectionAccounts: "CONTAS LOCAIS", SectionPortfolio: "CARTEIRA", SectionSend: "ENVIAR FUNDOS"}},
	{Code: "fr", Name: "French", NativeName: "Français", Terms: walletLanguageTerms{Portfolio: "Portefeuille", Send: "Envoyer", Accounts: "Comptes", Phone: "Téléphone TKM", Email: "E-mail", Kings: "Rois tournants", Refresh: "Actualiser", Language: "Langue", Exit: "Quitter", Select: "Sélectionnez une option", Closed: "Portefeuille fermé.", LanguageUI: "Langue", AutoDetect: "Détection système", Current: "Langue actuelle", Saved: "Langue enregistrée", Change: "Changer de langue", SectionPhone: "TÉLÉPHONE TKM", SectionEmail: "E-MAIL", SectionKings: "ROIS TOURNANTS", SectionAccounts: "COMPTES LOCAUX", SectionPortfolio: "PORTEFEUILLE", SectionSend: "ENVOYER DES FONDS"}},
	{Code: "de", Name: "German", NativeName: "Deutsch", Terms: walletLanguageTerms{Portfolio: "Portfolio", Send: "Senden", Accounts: "Konten", Phone: "TKM-Telefon", Email: "E-Mail", Kings: "Rotierende Könige", Refresh: "Aktualisieren", Language: "Sprache", Exit: "Beenden", Select: "Option auswählen", Closed: "Wallet geschlossen.", LanguageUI: "Sprache", AutoDetect: "Automatisch aus dem System", Current: "Aktuelle Sprache", Saved: "Sprache gespeichert", Change: "Sprache ändern", SectionPhone: "TKM-TELEFON", SectionEmail: "E-MAIL", SectionKings: "ROTIERENDE KÖNIGE", SectionAccounts: "LOKALE KONTEN", SectionPortfolio: "PORTFOLIO", SectionSend: "GELD SENDEN"}},
	{Code: "ar", Name: "Arabic", NativeName: "العربية", Terms: walletLanguageTerms{Portfolio: "المحفظة", Send: "إرسال", Accounts: "الحسابات", Phone: "هاتف TKM", Email: "البريد الإلكتروني", Kings: "الملوك المتناوبون", Refresh: "تحديث", Language: "اللغة", Exit: "خروج", Select: "اختر خياراً", Closed: "أُغلقت المحفظة.", LanguageUI: "اللغة", AutoDetect: "اكتشاف من النظام", Current: "اللغة الحالية", Saved: "تم حفظ اللغة", Change: "تغيير اللغة", SectionPhone: "هاتف TKM", SectionEmail: "البريد الإلكتروني", SectionKings: "الملوك المتناوبون", SectionAccounts: "الحسابات المحلية", SectionPortfolio: "المحفظة", SectionSend: "إرسال الأموال"}},
	{Code: "hi", Name: "Hindi", NativeName: "हिन्दी", Terms: walletLanguageTerms{Portfolio: "पोर्टफोलियो", Send: "भेजें", Accounts: "खाते", Phone: "TKM फ़ोन", Email: "ईमेल", Kings: "रोटेटिंग किंग", Refresh: "रीफ़्रेश", Language: "भाषा", Exit: "बाहर निकलें", Select: "विकल्प चुनें", Closed: "वॉलेट बंद है।", LanguageUI: "भाषा", AutoDetect: "सिस्टम से स्वतः पहचानें", Current: "वर्तमान भाषा", Saved: "भाषा सहेजी गई", Change: "भाषा बदलें", SectionPhone: "TKM फ़ोन", SectionEmail: "ईमेल", SectionKings: "रोटेटिंग किंग", SectionAccounts: "स्थानीय खाते", SectionPortfolio: "पोर्टफोलियो", SectionSend: "धन भेजें"}},
	{Code: "id", Name: "Indonesian", NativeName: "Bahasa Indonesia", Terms: walletLanguageTerms{Portfolio: "Portofolio", Send: "Kirim", Accounts: "Akun", Phone: "Telepon TKM", Email: "Email", Kings: "Raja bergilir", Refresh: "Muat ulang", Language: "Bahasa", Exit: "Keluar", Select: "Pilih opsi", Closed: "Dompet ditutup.", LanguageUI: "Bahasa", AutoDetect: "Deteksi dari sistem", Current: "Bahasa saat ini", Saved: "Bahasa disimpan", Change: "Ubah bahasa", SectionPhone: "TELEPON TKM", SectionEmail: "EMAIL", SectionKings: "RAJA BERGILIR", SectionAccounts: "AKUN LOKAL", SectionPortfolio: "PORTOFOLIO", SectionSend: "KIRIM DANA"}},
	{Code: "tr", Name: "Turkish", NativeName: "Türkçe", Terms: walletLanguageTerms{Portfolio: "Portföy", Send: "Gönder", Accounts: "Hesaplar", Phone: "TKM Telefon", Email: "E-posta", Kings: "Dönen krallar", Refresh: "Yenile", Language: "Dil", Exit: "Çıkış", Select: "Bir seçenek seçin", Closed: "Cüzdan kapatıldı.", LanguageUI: "Dil", AutoDetect: "Sistemden otomatik algıla", Current: "Geçerli dil", Saved: "Dil kaydedildi", Change: "Dili değiştir", SectionPhone: "TKM TELEFON", SectionEmail: "E-POSTA", SectionKings: "DÖNEN KRALLAR", SectionAccounts: "YEREL HESAPLAR", SectionPortfolio: "PORTFÖY", SectionSend: "PARA GÖNDER"}},
	{Code: "vi", Name: "Vietnamese", NativeName: "Tiếng Việt", Terms: walletLanguageTerms{Portfolio: "Danh mục", Send: "Gửi", Accounts: "Tài khoản", Phone: "Điện thoại TKM", Email: "Email", Kings: "Vua luân phiên", Refresh: "Làm mới", Language: "Ngôn ngữ", Exit: "Thoát", Select: "Chọn một tùy chọn", Closed: "Đã đóng ví.", LanguageUI: "Ngôn ngữ", AutoDetect: "Tự động phát hiện từ hệ thống", Current: "Ngôn ngữ hiện tại", Saved: "Đã lưu ngôn ngữ", Change: "Đổi ngôn ngữ", SectionPhone: "ĐIỆN THOẠI TKM", SectionEmail: "EMAIL", SectionKings: "VUA LUÂN PHIÊN", SectionAccounts: "TÀI KHOẢN CỤC BỘ", SectionPortfolio: "DANH MỤC", SectionSend: "GỬI TIỀN"}},
	{Code: "it", Name: "Italian", NativeName: "Italiano", Terms: walletLanguageTerms{Portfolio: "Portafoglio", Send: "Invia", Accounts: "Account", Phone: "Telefono TKM", Email: "Email", Kings: "Re rotanti", Refresh: "Aggiorna", Language: "Lingua", Exit: "Esci", Select: "Seleziona un'opzione", Closed: "Portafoglio chiuso.", LanguageUI: "Lingua", AutoDetect: "Rileva dal sistema", Current: "Lingua corrente", Saved: "Lingua salvata", Change: "Cambia lingua", SectionPhone: "TELEFONO TKM", SectionEmail: "EMAIL", SectionKings: "RE ROTANTI", SectionAccounts: "ACCOUNT LOCALI", SectionPortfolio: "PORTAFOGLIO", SectionSend: "INVIA FONDI"}},
	{Code: "nl", Name: "Dutch", NativeName: "Nederlands", Terms: walletLanguageTerms{Portfolio: "Portfolio", Send: "Verzenden", Accounts: "Accounts", Phone: "TKM-telefoon", Email: "E-mail", Kings: "Roterende koningen", Refresh: "Vernieuwen", Language: "Taal", Exit: "Afsluiten", Select: "Kies een optie", Closed: "Portemonnee gesloten.", LanguageUI: "Taal", AutoDetect: "Automatisch uit systeem", Current: "Huidige taal", Saved: "Taal opgeslagen", Change: "Taal wijzigen", SectionPhone: "TKM-TELEFOON", SectionEmail: "E-MAIL", SectionKings: "ROTERENDE KONINGEN", SectionAccounts: "LOKALE ACCOUNTS", SectionPortfolio: "PORTFOLIO", SectionSend: "GELD VERZENDEN"}},
	{Code: "pl", Name: "Polish", NativeName: "Polski", Terms: walletLanguageTerms{Portfolio: "Portfel", Send: "Wyślij", Accounts: "Konta", Phone: "Telefon TKM", Email: "E-mail", Kings: "Rotujący królowie", Refresh: "Odśwież", Language: "Język", Exit: "Wyjdź", Select: "Wybierz opcję", Closed: "Portfel zamknięty.", LanguageUI: "Język", AutoDetect: "Wykryj z systemu", Current: "Bieżący język", Saved: "Język zapisany", Change: "Zmień język", SectionPhone: "TELEFON TKM", SectionEmail: "E-MAIL", SectionKings: "ROTUJĄCY KRÓLOWIE", SectionAccounts: "KONTA LOKALNE", SectionPortfolio: "PORTFEL", SectionSend: "WYŚLIJ ŚRODKI"}},
	{Code: "uk", Name: "Ukrainian", NativeName: "Українська", Terms: walletLanguageTerms{Portfolio: "Портфель", Send: "Надіслати", Accounts: "Рахунки", Phone: "Телефон TKM", Email: "Е-пошта", Kings: "Королі ротації", Refresh: "Оновити", Language: "Мова", Exit: "Вийти", Select: "Виберіть пункт", Closed: "Гаманець закрито.", LanguageUI: "Мова", AutoDetect: "Визначити із системи", Current: "Поточна мова", Saved: "Мову збережено", Change: "Змінити мову", SectionPhone: "ТЕЛЕФОН TKM", SectionEmail: "Е-ПОШТА", SectionKings: "КОРОЛІ РОТАЦІЇ", SectionAccounts: "ЛОКАЛЬНІ РАХУНКИ", SectionPortfolio: "ПОРТФЕЛЬ", SectionSend: "НАДІСЛАТИ КОШТИ"}},
	{Code: "th", Name: "Thai", NativeName: "ไทย", Terms: walletLanguageTerms{Portfolio: "พอร์ตโฟลิโอ", Send: "ส่ง", Accounts: "บัญชี", Phone: "โทรศัพท์ TKM", Email: "อีเมล", Kings: "กษัตริย์หมุนเวียน", Refresh: "รีเฟรช", Language: "ภาษา", Exit: "ออก", Select: "เลือกตัวเลือก", Closed: "ปิดกระเป๋าแล้ว", LanguageUI: "ภาษา", AutoDetect: "ตรวจจับจากระบบ", Current: "ภาษาปัจจุบัน", Saved: "บันทึกภาษาแล้ว", Change: "เปลี่ยนภาษา", SectionPhone: "โทรศัพท์ TKM", SectionEmail: "อีเมล", SectionKings: "กษัตริย์หมุนเวียน", SectionAccounts: "บัญชีท้องถิ่น", SectionPortfolio: "พอร์ตโฟลิโอ", SectionSend: "ส่งเงิน"}},
	{Code: "bn", Name: "Bengali", NativeName: "বাংলা", Terms: walletLanguageTerms{Portfolio: "পোর্টফোলিও", Send: "পাঠান", Accounts: "অ্যাকাউন্ট", Phone: "TKM ফোন", Email: "ইমেল", Kings: "ঘূর্ণায়মান রাজা", Refresh: "রিফ্রেশ", Language: "ভাষা", Exit: "প্রস্থান", Select: "একটি বিকল্প বেছে নিন", Closed: "ওয়ালেট বন্ধ হয়েছে।", LanguageUI: "ভাষা", AutoDetect: "সিস্টেম থেকে স্বয়ংক্রিয় শনাক্ত", Current: "বর্তমান ভাষা", Saved: "ভাষা সংরক্ষিত", Change: "ভাষা পরিবর্তন", SectionPhone: "TKM ফোন", SectionEmail: "ইমেল", SectionKings: "ঘূর্ণায়মান রাজা", SectionAccounts: "স্থানীয় অ্যাকাউন্ট", SectionPortfolio: "পোর্টফোলিও", SectionSend: "অর্থ পাঠান"}},
}

var walletDaemonTranslations = map[string]map[string]string{
	"zh": {"daemon.starting": "正在启动 RandomX 主网…", "daemon.language": "已选择守护进程界面语言"},
	"ru": {"daemon.starting": "Запуск основной сети RandomX…", "daemon.language": "Выбран язык интерфейса узла"},
	"en": {"daemon.starting": "Starting Geth on RandomX mainnet...", "daemon.language": "Daemon interface language selected"},
	"ja": {"daemon.starting": "RandomX メインネットを起動しています…", "daemon.language": "デーモンのインターフェース言語を選択しました"},
	"ko": {"daemon.starting": "RandomX 메인넷을 시작합니다…", "daemon.language": "데몬 인터페이스 언어가 선택되었습니다"},
	"es": {"daemon.starting": "Iniciando la red principal de RandomX…", "daemon.language": "Idioma de interfaz del demonio seleccionado"},
	"pt": {"daemon.starting": "Iniciando a rede principal RandomX…", "daemon.language": "Idioma da interface do daemon selecionado"},
	"fr": {"daemon.starting": "Démarrage du réseau principal RandomX…", "daemon.language": "Langue de l’interface du démon sélectionnée"},
	"de": {"daemon.starting": "RandomX-Mainnet wird gestartet…", "daemon.language": "Sprache der Daemon-Oberfläche ausgewählt"},
	"ar": {"daemon.starting": "جارٍ تشغيل شبكة RandomX الرئيسية…", "daemon.language": "تم اختيار لغة واجهة الخدمة"},
	"hi": {"daemon.starting": "RandomX मेननेट शुरू हो रहा है…", "daemon.language": "डेमन इंटरफ़ेस भाषा चुनी गई"},
	"id": {"daemon.starting": "Memulai jaringan utama RandomX…", "daemon.language": "Bahasa antarmuka daemon dipilih"},
	"tr": {"daemon.starting": "RandomX ana ağı başlatılıyor…", "daemon.language": "Daemon arayüz dili seçildi"},
	"vi": {"daemon.starting": "Đang khởi động mạng chính RandomX…", "daemon.language": "Đã chọn ngôn ngữ giao diện daemon"},
	"it": {"daemon.starting": "Avvio della mainnet RandomX…", "daemon.language": "Lingua dell’interfaccia del daemon selezionata"},
	"nl": {"daemon.starting": "RandomX-mainnet wordt gestart…", "daemon.language": "Taal van de daemoninterface geselecteerd"},
	"pl": {"daemon.starting": "Uruchamianie sieci głównej RandomX…", "daemon.language": "Wybrano język interfejsu demona"},
	"uk": {"daemon.starting": "Запуск основної мережі RandomX…", "daemon.language": "Вибрано мову інтерфейсу вузла"},
	"th": {"daemon.starting": "กำลังเริ่มเครือข่ายหลัก RandomX…", "daemon.language": "เลือกภาษาสำหรับอินเทอร์เฟซของเดมอนแล้ว"},
	"bn": {"daemon.starting": "RandomX মেইননেট শুরু হচ্ছে…", "daemon.language": "ডেমন ইন্টারফেসের ভাষা নির্বাচিত হয়েছে"},
}

func walletLanguageByCode(code string) walletLanguage {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, language := range walletLanguageCatalog {
		if language.Code == code {
			return language
		}
	}
	return walletLanguageByCode("en")
}

func detectWalletLanguage() walletLanguage {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.ToLower(os.Getenv(name))
		for _, language := range walletLanguageCatalog {
			if strings.HasPrefix(value, language.Code+"_") || strings.HasPrefix(value, language.Code+"-") || value == language.Code {
				return language
			}
		}
	}
	return walletLanguageByCode("en")
}

// walletLogCatalog translates the high-value daemon lifecycle messages that
// operators see while a node starts, synchronizes, and shuts down. Structured
// fields (hashes, heights, endpoints, and errors) are preserved verbatim.
// Unknown diagnostic messages intentionally keep their canonical English text
// so support tooling and peer reports remain searchable across locales.
var walletLogCatalog = map[string]map[string]string{
	"zh": {
		"Starting peer-to-peer node":                          "正在启动点对点网络",
		"New local node record":                               "已创建本地节点记录",
		"Started P2P networking":                              "点对点网络已启动",
		"IPC endpoint opened":                                 "IPC 端点已打开",
		"HTTP server started":                                 "HTTP 服务器已启动",
		"WebSocket enabled":                                   "WebSocket 已启用",
		"Loaded local transaction journal":                    "已加载本地交易日志",
		"Tkmchain backend started with RandomX consensus":     "TKMChain RandomX 共识后端已启动",
		"Gtkm node is running; waiting for shutdown signal":   "Gtkm 节点正在运行，等待关闭信号",
		"Looking for peers":                                   "正在查找节点",
		"Synchronisation failed, retrying":                    "同步失败，正在重试",
		"Received interrupt, saving state before shutdown...": "收到中断信号，正在保存状态后关闭…",
		"Closing node after interrupt":                        "正在中断后关闭节点",
		"Smartcard socket not found, disabling":               "未找到智能卡套接字，已禁用",
		"RandomX mining enabled":                              "RandomX 挖矿已启用",
		"King configuration loaded":                           "王配置已加载",
		"TKM shielded prover started":                         "TKM 隐私证明器已启动",
	},
	"ru": {
		"Starting peer-to-peer node":                          "Запуск однорангового узла",
		"New local node record":                               "Создана запись локального узла",
		"Started P2P networking":                              "P2P-сеть запущена",
		"IPC endpoint opened":                                 "Конечная точка IPC открыта",
		"HTTP server started":                                 "HTTP-сервер запущен",
		"WebSocket enabled":                                   "WebSocket включён",
		"Loaded local transaction journal":                    "Локальный журнал транзакций загружен",
		"Tkmchain backend started with RandomX consensus":     "Бэкенд консенсуса TKMChain RandomX запущен",
		"Gtkm node is running; waiting for shutdown signal":   "Узел Gtkm работает и ожидает сигнал завершения",
		"Looking for peers":                                   "Поиск узлов",
		"Synchronisation failed, retrying":                    "Синхронизация не удалась, повтор",
		"Received interrupt, saving state before shutdown...": "Получено прерывание, сохранение состояния перед завершением…",
		"Closing node after interrupt":                        "Остановка узла после прерывания",
		"Smartcard socket not found, disabling":               "Сокет смарт-карты не найден, функция отключена",
		"RandomX mining enabled":                              "Майнинг RandomX включён",
		"King configuration loaded":                           "Конфигурация королей загружена",
		"TKM shielded prover started":                         "Прoвер TKM Shielded запущен",
	},
	"ja": {
		"Starting peer-to-peer node":                          "ピアツーピアノードを起動しています",
		"New local node record":                               "ローカルノードレコードを作成しました",
		"Started P2P networking":                              "P2P ネットワークを起動しました",
		"IPC endpoint opened":                                 "IPC エンドポイントを開きました",
		"HTTP server started":                                 "HTTP サーバーを起動しました",
		"WebSocket enabled":                                   "WebSocket を有効にしました",
		"Loaded local transaction journal":                    "ローカルトランザクションジャーナルを読み込みました",
		"Tkmchain backend started with RandomX consensus":     "TKMChain RandomX コンセンサスバックエンドを起動しました",
		"Gtkm node is running; waiting for shutdown signal":   "Gtkm ノードは実行中です。終了シグナルを待っています",
		"Looking for peers":                                   "ピアを検索しています",
		"Synchronisation failed, retrying":                    "同期に失敗しました。再試行します",
		"Received interrupt, saving state before shutdown...": "割り込みを受信しました。終了前に状態を保存しています…",
		"Closing node after interrupt":                        "割り込み後にノードを終了しています",
		"Smartcard socket not found, disabling":               "スマートカードソケットが見つからないため無効にしました",
		"RandomX mining enabled":                              "RandomX マイニングを有効にしました",
		"King configuration loaded":                           "キング設定を読み込みました",
		"TKM shielded prover started":                         "TKM シールドプルーバーを起動しました",
	},
	"ko": {
		"Starting peer-to-peer node":                          "피어 투 피어 노드를 시작합니다",
		"New local node record":                               "로컬 노드 레코드가 생성되었습니다",
		"Started P2P networking":                              "P2P 네트워크가 시작되었습니다",
		"IPC endpoint opened":                                 "IPC 엔드포인트가 열렸습니다",
		"HTTP server started":                                 "HTTP 서버가 시작되었습니다",
		"WebSocket enabled":                                   "WebSocket이 활성화되었습니다",
		"Loaded local transaction journal":                    "로컬 트랜잭션 저널을 불러왔습니다",
		"Tkmchain backend started with RandomX consensus":     "TKMChain RandomX 합의 백엔드가 시작되었습니다",
		"Gtkm node is running; waiting for shutdown signal":   "Gtkm 노드가 실행 중이며 종료 신호를 기다립니다",
		"Looking for peers":                                   "피어를 찾는 중입니다",
		"Synchronisation failed, retrying":                    "동기화에 실패하여 다시 시도합니다",
		"Received interrupt, saving state before shutdown...": "중단 신호를 받아 종료 전에 상태를 저장합니다…",
		"Closing node after interrupt":                        "중단 후 노드를 종료합니다",
		"Smartcard socket not found, disabling":               "스마트카드 소켓을 찾지 못해 비활성화했습니다",
		"RandomX mining enabled":                              "RandomX 채굴이 활성화되었습니다",
		"King configuration loaded":                           "킹 설정을 불러왔습니다",
		"TKM shielded prover started":                         "TKM 실드 프로버가 시작되었습니다",
	},
}

var walletProtocolLogTranslations = map[string]map[string]string{
	"zh": {"Phone hardfork difficulty adjustment": "TKM 电话硬分叉难度调整", "Egypt emergency difficulty adjustment applied": "已应用埃及紧急难度调整", "Emergency difficulty adjustment applied": "已应用紧急难度调整"},
	"ru": {"Phone hardfork difficulty adjustment": "Корректировка сложности хардфорка Phone", "Egypt emergency difficulty adjustment applied": "Применена экстренная корректировка сложности Египта", "Emergency difficulty adjustment applied": "Применена экстренная корректировка сложности"},
	"en": {"Phone hardfork difficulty adjustment": "Phone hardfork difficulty adjustment", "Egypt emergency difficulty adjustment applied": "Egypt emergency difficulty adjustment applied", "Emergency difficulty adjustment applied": "Emergency difficulty adjustment applied"},
	"ja": {"Phone hardfork difficulty adjustment": "Phone ハードフォークの難易度調整", "Egypt emergency difficulty adjustment applied": "エジプト緊急難易度調整を適用しました", "Emergency difficulty adjustment applied": "緊急難易度調整を適用しました"},
	"ko": {"Phone hardfork difficulty adjustment": "Phone 하드포크 난이도 조정", "Egypt emergency difficulty adjustment applied": "이집트 긴급 난이도 조정을 적용했습니다", "Emergency difficulty adjustment applied": "긴급 난이도 조정을 적용했습니다"},
	"es": {"Phone hardfork difficulty adjustment": "Ajuste de dificultad del hard fork de Phone", "Egypt emergency difficulty adjustment applied": "Se aplicó el ajuste de dificultad de emergencia de Egypt", "Emergency difficulty adjustment applied": "Se aplicó el ajuste de dificultad de emergencia"},
	"pt": {"Phone hardfork difficulty adjustment": "Ajuste de dificuldade do hard fork do Phone", "Egypt emergency difficulty adjustment applied": "Ajuste de dificuldade de emergência do Egypt aplicado", "Emergency difficulty adjustment applied": "Ajuste de dificuldade de emergência aplicado"},
	"fr": {"Phone hardfork difficulty adjustment": "Ajustement de difficulté du hard fork Phone", "Egypt emergency difficulty adjustment applied": "Ajustement de difficulté d’urgence d’Egypt appliqué", "Emergency difficulty adjustment applied": "Ajustement de difficulté d’urgence appliqué"},
	"de": {"Phone hardfork difficulty adjustment": "Schwierigkeitsanpassung des Phone-Hardforks", "Egypt emergency difficulty adjustment applied": "Notfallanpassung der Egypt-Schwierigkeit angewendet", "Emergency difficulty adjustment applied": "Notfallanpassung der Schwierigkeit angewendet"},
	"ar": {"Phone hardfork difficulty adjustment": "ضبط صعوبة الانقسام الصلب للهاتف", "Egypt emergency difficulty adjustment applied": "تم تطبيق ضبط صعوبة الطوارئ في Egypt", "Emergency difficulty adjustment applied": "تم تطبيق ضبط صعوبة الطوارئ"},
	"hi": {"Phone hardfork difficulty adjustment": "Phone हार्ड फोर्क कठिनाई समायोजन", "Egypt emergency difficulty adjustment applied": "Egypt आपातकालीन कठिनाई समायोजन लागू किया गया", "Emergency difficulty adjustment applied": "आपातकालीन कठिनाई समायोजन लागू किया गया"},
	"id": {"Phone hardfork difficulty adjustment": "Penyesuaian tingkat kesulitan hard fork Phone", "Egypt emergency difficulty adjustment applied": "Penyesuaian tingkat kesulitan darurat Egypt diterapkan", "Emergency difficulty adjustment applied": "Penyesuaian tingkat kesulitan darurat diterapkan"},
	"tr": {"Phone hardfork difficulty adjustment": "Phone hard fork zorluk ayarı", "Egypt emergency difficulty adjustment applied": "Egypt acil zorluk ayarı uygulandı", "Emergency difficulty adjustment applied": "Acil zorluk ayarı uygulandı"},
	"vi": {"Phone hardfork difficulty adjustment": "Điều chỉnh độ khó hard fork Phone", "Egypt emergency difficulty adjustment applied": "Đã áp dụng điều chỉnh độ khó khẩn cấp của Egypt", "Emergency difficulty adjustment applied": "Đã áp dụng điều chỉnh độ khó khẩn cấp"},
	"it": {"Phone hardfork difficulty adjustment": "Adeguamento della difficoltà dell’hard fork Phone", "Egypt emergency difficulty adjustment applied": "Applicato l’adeguamento di emergenza della difficoltà Egypt", "Emergency difficulty adjustment applied": "Applicato l’adeguamento di emergenza della difficoltà"},
	"nl": {"Phone hardfork difficulty adjustment": "Moeilijkheidsaanpassing van de Phone-hard fork", "Egypt emergency difficulty adjustment applied": "Egypt-noodaanpassing van de moeilijkheid toegepast", "Emergency difficulty adjustment applied": "Noodaanpassing van de moeilijkheid toegepast"},
	"pl": {"Phone hardfork difficulty adjustment": "Dostosowanie trudności hard forka Phone", "Egypt emergency difficulty adjustment applied": "Zastosowano awaryjne dostosowanie trudności Egypt", "Emergency difficulty adjustment applied": "Zastosowano awaryjne dostosowanie trudności"},
	"uk": {"Phone hardfork difficulty adjustment": "Налаштування складності хардфорку Phone", "Egypt emergency difficulty adjustment applied": "Застосовано аварійне налаштування складності Egypt", "Emergency difficulty adjustment applied": "Застосовано аварійне налаштування складності"},
	"th": {"Phone hardfork difficulty adjustment": "การปรับความยากของฮาร์ดฟอร์ก Phone", "Egypt emergency difficulty adjustment applied": "ใช้การปรับความยากฉุกเฉินของ Egypt แล้ว", "Emergency difficulty adjustment applied": "ใช้การปรับความยากฉุกเฉินแล้ว"},
	"bn": {"Phone hardfork difficulty adjustment": "Phone হার্ড ফর্কের কঠিনতা সমন্বয়", "Egypt emergency difficulty adjustment applied": "Egypt জরুরি কঠিনতা সমন্বয় প্রয়োগ করা হয়েছে", "Emergency difficulty adjustment applied": "জরুরি কঠিনতা সমন্বয় প্রয়োগ করা হয়েছে"},
}

type walletLogTranslationHandler struct {
	next     slog.Handler
	language string
}

func (h *walletLogTranslationHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *walletLogTranslationHandler) Handle(ctx context.Context, record slog.Record) error {
	translated := walletLogCatalog[h.language][record.Message]
	if translated == "" {
		translated = walletProtocolLogTranslations[h.language][record.Message]
	}
	if translated != "" {
		record.Message = translated
	}
	return h.next.Handle(ctx, record)
}

func (h *walletLogTranslationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &walletLogTranslationHandler{next: h.next.WithAttrs(attrs), language: h.language}
}

func (h *walletLogTranslationHandler) WithGroup(name string) slog.Handler {
	return &walletLogTranslationHandler{next: h.next.WithGroup(name), language: h.language}
}

func installWalletLogTranslation(language walletLanguage) {
	root := log.Root()
	if _, alreadyWrapped := root.Handler().(*walletLogTranslationHandler); alreadyWrapped {
		return
	}
	log.SetDefault(log.NewLogger(&walletLogTranslationHandler{next: root.Handler(), language: language.Code}))
}

func walletText(key, fallback string) string {
	if translations := walletDaemonTranslations[walletActiveLanguage.Code]; translations != nil {
		if value := translations[key]; value != "" {
			return value
		}
	}
	terms := walletActiveLanguage.Terms
	var value string
	switch key {
	case "title":
		value = terms.Title
	case "subtitle":
		value = terms.Subtitle
	case "menu.portfolio":
		value = terms.Portfolio
	case "menu.send":
		value = terms.Send
	case "menu.accounts":
		value = terms.Accounts
	case "menu.phone":
		value = terms.Phone
	case "menu.email":
		value = terms.Email
	case "menu.kings":
		value = terms.Kings
	case "menu.refresh":
		value = terms.Refresh
	case "menu.language":
		value = terms.Language
	case "menu.migrate":
		value = map[string]string{
			"zh": "将 ECDSA 迁移到 ML-DSA-87", "ru": "Миграция ECDSA → ML-DSA-87",
			"ja": "ECDSA を ML-DSA-87 に移行", "ko": "ECDSA → ML-DSA-87 마이그레이션",
			"es": "Migrar ECDSA → ML-DSA-87", "pt": "Migrar ECDSA → ML-DSA-87",
			"fr": "Migrer ECDSA → ML-DSA-87", "de": "ECDSA → ML-DSA-87 migrieren",
			"ar": "ترحيل ECDSA إلى ML-DSA-87", "hi": "ECDSA → ML-DSA-87 माइग्रेट करें",
			"id": "Migrasikan ECDSA → ML-DSA-87", "tr": "ECDSA → ML-DSA-87 taşı",
			"vi": "Di chuyển ECDSA → ML-DSA-87", "it": "Migra ECDSA → ML-DSA-87",
			"nl": "ECDSA → ML-DSA-87 migreren", "pl": "Migruj ECDSA → ML-DSA-87",
			"uk": "Міграція ECDSA → ML-DSA-87", "th": "ย้าย ECDSA → ML-DSA-87",
			"bn": "ECDSA → ML-DSA-87 মাইগ্রেট করুন",
		}[walletActiveLanguage.Code]
	case "menu.exit":
		value = terms.Exit
	case "select":
		value = terms.Select
	case "closed":
		value = terms.Closed
	case "language.title":
		value = terms.LanguageUI
	case "language.auto":
		value = terms.AutoDetect
	case "language.detected":
		value = terms.Detected
	case "language.current":
		value = terms.Current
	case "language.saved":
		value = terms.Saved
	case "language.change":
		value = terms.Change
	case "section.phone":
		value = terms.SectionPhone
	case "section.email":
		value = terms.SectionEmail
	case "section.kings":
		value = terms.SectionKings
	case "section.accounts":
		value = terms.SectionAccounts
	case "section.portfolio":
		value = terms.SectionPortfolio
	case "section.send":
		value = terms.SectionSend
	}
	if value == "" {
		return fallback
	}
	return value
}

func walletLanguagePath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, "wallet-language.json")
}

func loadWalletLanguage(dataDir string) (walletLanguagePreference, bool) {
	path := walletLanguagePath(dataDir)
	if path == "" {
		return walletLanguagePreference{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return walletLanguagePreference{}, false
	}
	var preference walletLanguagePreference
	if json.Unmarshal(data, &preference) != nil || preference.Language == "" {
		return walletLanguagePreference{}, false
	}
	return preference, true
}

func saveWalletLanguage(dataDir string, preference walletLanguagePreference) error {
	path := walletLanguagePath(dataDir)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(preference, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func chooseWalletLanguage(reader *bufio.Reader) (walletLanguagePreference, walletLanguage, error) {
	fmt.Println("\n" + walletText("language.title", "LANGUAGE"))
	fmt.Println("  Choose a language, or choose auto-detect.")
	for index, language := range walletLanguageCatalog {
		fmt.Printf("  %2d) %s — %s\n", index+1, language.Name, language.NativeName)
	}
	fmt.Printf("   A) %s\n", walletText("language.auto", "Auto-detect from system"))
	choice, err := readWalletLine(reader, walletText("select", "Select an option"))
	if err != nil {
		return walletLanguagePreference{}, walletLanguage{}, err
	}
	choice = strings.TrimSpace(choice)
	if choice == "" || strings.EqualFold(choice, "a") {
		language := detectWalletLanguage()
		return walletLanguagePreference{Language: "auto"}, language, nil
	}
	var index int
	if _, err := fmt.Sscanf(choice, "%d", &index); err != nil || index < 1 || index > len(walletLanguageCatalog) {
		return walletLanguagePreference{}, walletLanguage{}, errors.New("invalid language selection")
	}
	language := walletLanguageCatalog[index-1]
	return walletLanguagePreference{Language: language.Code}, language, nil
}

func walletLanguageFromFlag(ctx *cli.Context) (walletLanguagePreference, walletLanguage, bool, error) {
	if !ctx.IsSet(walletLanguageFlag.Name) {
		return walletLanguagePreference{}, walletLanguage{}, false, nil
	}
	code := strings.ToLower(strings.TrimSpace(ctx.String(walletLanguageFlag.Name)))
	if code == "" || code == "auto" {
		return walletLanguagePreference{Language: "auto"}, detectWalletLanguage(), true, nil
	}
	language := walletLanguageByCode(code)
	if language.Code != code {
		return walletLanguagePreference{}, walletLanguage{}, true, fmt.Errorf("unsupported language %q", code)
	}
	return walletLanguagePreference{Language: language.Code}, language, true, nil
}

func walletLanguageDataDir(ctx *cli.Context) string {
	cfg := defaultNodeConfig()
	utils.SetDataDir(ctx, &cfg)
	return cfg.DataDir
}

func configureDaemonWalletLanguage(ctx *cli.Context) walletLanguage {
	if _, language, ok, err := walletLanguageFromFlag(ctx); ok {
		if err != nil {
			log.Warn("unsupported wallet language; using detected locale", "error", err)
			language = detectWalletLanguage()
		}
		walletActiveLanguage = language
		installWalletLogTranslation(language)
		return language
	}
	dataDir := walletLanguageDataDir(ctx)
	preference, saved := loadWalletLanguage(dataDir)
	language := detectWalletLanguage()
	if saved && preference.Language != "" && preference.Language != "auto" {
		candidate := walletLanguageByCode(preference.Language)
		if candidate.Code == preference.Language {
			language = candidate
		}
	}
	walletActiveLanguage = language
	installWalletLogTranslation(language)
	return language
}

func configureWalletLanguage(reader *bufio.Reader, dataDir string) (walletLanguagePreference, walletLanguage, error) {
	if preference, ok := loadWalletLanguage(dataDir); ok {
		if preference.Language == "auto" {
			return preference, detectWalletLanguage(), nil
		}
		return preference, walletLanguageByCode(preference.Language), nil
	}
	preference, language, err := chooseWalletLanguage(reader)
	if err != nil {
		return walletLanguagePreference{}, walletLanguage{}, err
	}
	if saveErr := saveWalletLanguage(dataDir, preference); saveErr != nil {
		fmt.Printf("  Could not save language preference: %v\n", saveErr)
	}
	return preference, language, nil
}
