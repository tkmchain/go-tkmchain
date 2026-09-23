package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func walletText(key, fallback string) string {
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
