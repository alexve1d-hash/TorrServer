package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/torr/utils"

	"github.com/alexflint/go-arg"
	"github.com/pkg/browser"

	"server"
	"server/docs"
	"server/log"
	"server/settings"
	"server/torr"
	"server/version"
)

type args struct {
	Port        string `arg:"-p" help:"web server port (default 8090)"`
	IP          string `arg:"-i" help:"web server addr (default empty)"`
	Ssl         bool   `help:"enables https"`
	SslPort     string `help:"web server ssl port, If not set, will be set to default 8091 or taken from db(if stored previously). Accepted if --ssl enabled."`
	SslCert     string `help:"path to ssl cert file. If not set, will be taken from db(if stored previously) or default self-signed certificate/key will be generated. Accepted if --ssl enabled."`
	SslKey      string `help:"path to ssl key file. If not set, will be taken from db(if stored previously) or default self-signed certificate/key will be generated. Accepted if --ssl enabled."`
	Path        string `arg:"-d" help:"database and config dir path"`
	LogPath     string `arg:"-l" help:"server log file path"`
	WebLogPath  string `arg:"-w" help:"web access log file path"`
	RDB         bool   `arg:"-r" help:"start in read-only DB mode"`
	HttpAuth    bool   `arg:"-a" help:"enable http auth on all requests"`
	DontKill    bool   `arg:"-k" help:"don't kill server on signal"`
	UI          bool   `arg:"-u" help:"open torrserver page in browser"`
	TorrentsDir string `arg:"-t" help:"autoload torrents from dir"`
	TorrentAddr string `help:"Torrent client address, like 127.0.0.1:1337 (default :PeersListenPort)"`
	PubIPv4     string `arg:"-4" help:"set public IPv4 addr"`
	PubIPv6     string `arg:"-6" help:"set public IPv6 addr"`
	SearchWA    bool   `arg:"-s" help:"search without auth"`
	MaxSize     string `arg:"-m" help:"max allowed stream size (in Bytes)"`
	TGToken     string `arg:"-T" help:"telegram bot token"`
	FusePath    string `arg:"-f" help:"fuse mount path"`
	WebDAV      bool   `help:"web dav enable"`
	ProxyURL    string `help:"proxy URL for BitTorrent traffic (http, socks4, socks5, socks5h), e.g. socks5://user:password@127.0.0.1:8080"`
	ProxyMode   string `help:"proxy mode: tracker (only HTTP trackers, default), peers (only peer connections), or full (all traffic)"`
	ForceHTTPS  bool   `arg:"--force-https" help:"redirect all HTTP requests to HTTPS (requires --ssl)"`
}

func (args) Version() string {
	return "TorrServer " + version.Version
}

var params args

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	arg.MustParse(&params)

	if params.Path == "" {
		params.Path, _ = os.Getwd()
	}

	if params.Port == "" {
		params.Port = "8090"
	}

	settings.Path = params.Path
	settings.HttpAuth = params.HttpAuth
	log.Init(params.LogPath, params.WebLogPath)

	fmt.Println("=========== START ===========")
	fmt.Println("TorrServer", version.Version+",", runtime.Version()+",", "CPU Num:", runtime.NumCPU())
	if params.HttpAuth {
		log.TLogln("Use HTTP Auth file", settings.Path+"/accs.db")
	}
	if params.RDB {
		log.TLogln("Running in Read-only DB mode!")
	}
	docs.SwaggerInfo.Version = version.Version

	// Simple Usage:
	dnsResolve()

	// Advanced Usage:
	// config := DNSConfig{
	//     PrimaryServers: []string{"1.1.1.1:53", "8.8.8.8:53"},
	//     Timeout:        3 * time.Second,
	// }
	// checker := NewDNSChecker(config)
	// // Perform DNS lookup with automatic fallback
	// addrs, err := checker.LookupHostWithFallback("themoviedb.org")
	// if err != nil {
	//     log.TLogln("DNS lookup failed:", err)
	// } else {
	// 	fmt.Println("DNS resolved:", addrs)
	// }

	Preconfig(params.DontKill)

	if params.UI {
		go func() {
			time.Sleep(time.Second)
			if params.Ssl {
				browser.OpenURL("https://127.0.0.1:" + params.SslPort)
			} else {
				browser.OpenURL("http://127.0.0.1:" + params.Port)
			}
		}()
	}

	if params.TorrentAddr != "" {
		settings.TorAddr = params.TorrentAddr
	}

	if params.PubIPv4 != "" {
		settings.PubIPv4 = params.PubIPv4
	}

	if params.PubIPv6 != "" {
		settings.PubIPv6 = params.PubIPv6
	}

	if params.TorrentsDir != "" {
		go watchTDir(params.TorrentsDir)
	}

	if params.MaxSize != "" {
		maxSize, err := strconv.ParseInt(params.MaxSize, 10, 64)
		if err == nil {
			settings.MaxSize = maxSize
		}
	}

	if params.ProxyURL != "" && params.ProxyMode == "" {
		params.ProxyMode = "tracker" // default
	}
	if params.ProxyMode != "" && params.ProxyMode != "tracker" && params.ProxyMode != "peers" && params.ProxyMode != "full" {
		log.TLogln("Invalid proxy mode, using default 'tracker'")
		params.ProxyMode = "tracker"
	}

	settings.Args = &settings.ExecArgs{
		Port:        params.Port,
		IP:          params.IP,
		Ssl:         params.Ssl,
		SslPort:     params.SslPort,
		SslCert:     params.SslCert,
		SslKey:      params.SslKey,
		Path:        params.Path,
		LogPath:     params.LogPath,
		WebLogPath:  params.WebLogPath,
		RDB:         params.RDB,
		HttpAuth:    params.HttpAuth,
		DontKill:    params.DontKill,
		UI:          params.UI,
		TorrentsDir: params.TorrentsDir,
		TorrentAddr: params.TorrentAddr,
		PubIPv4:     params.PubIPv4,
		PubIPv6:     params.PubIPv6,
		SearchWA:    params.SearchWA,
		MaxSize:     params.MaxSize,
		TGToken:     params.TGToken,
		FusePath:    params.FusePath,
		WebDAV:      params.WebDAV,
		ProxyURL:    params.ProxyURL,
		ProxyMode:   params.ProxyMode,
		ForceHTTPS:  params.ForceHTTPS,
	}

	if params.ProxyURL != "" {
		log.TLogln("Proxy configured from CLI:", params.ProxyURL, "mode:", settings.Args.ProxyMode)
	}

	if params.ForceHTTPS && !params.Ssl {
		log.TLogln("Error: --force-https requires --ssl")
		os.Exit(1)
	}

	server.Start()
	log.TLogln(server.WaitServer())
	log.Close()
	time.Sleep(time.Second * 3)
	os.Exit(0)
}

func watchTDir(dir string) {             //Объявление функции watchTDir, принимающей путь к отслеживаемой папке dir в виде строки.
	time.Sleep(5 * time.Second)  //Задержка в 5 секунд при старте сервера, чтобы дать всем основным компонентам (БД, сеть) полностью инициализироваться.
	path, err := filepath.Abs(dir)   //Попытка преобразовать переданный путь dir в абсолютный (полный) путь в файловой системе.
	if err != nil {
		path = dir
	}                                  //Если при получении абсолютного пути возникла ошибка, используется исходный относительный путь.
	for {                                    //Запуск бесконечного цикла для непрерывного фонового мониторинга указанной директории.
		files, err := os.ReadDir(path)         // Чтение списка всех файлов и папок, находящихся по пути path.
		if err == nil {                        //Проверка: если чтение директории прошло успешно (без ошибок доступа или отсутствия папки), переходим к обработке элементов.
			for _, file := range files {       //Цикл перебора каждого найденного элемента file в полученном списке файлов.
				func() {                       //Открытие анонимной функции для изоляции контекста обработки каждого отдельного файла. Это критически важно для безопасной работы защиты recover и корректного использования оператора return (который внутри анонимной функции действует как continue для основного цикла).
					defer func() {              //Отложенный вызов функции защиты, которая гарантированно сработает в самом конце выполнения анонимной функции (даже в случае критического сбоя/паники).
						if r := recover(); r != nil {         //Перехват критической ошибки (panic), если она возникнет глубоко внутри логики TorrServer при обработке конкретного торрента.
							log.TLogln("Recovered from panic in watchTDir:", r)        //Запись сообщения о перехваченной панике в лог сервера, что предотвращает падение и закрытие всего приложения TorrServer.
						}
					}()                                                                          //Закрытие и немедленная регистрация отложенной функции защиты defer.

					filename := filepath.Join(path, file.Name())                                // Сборка полного пути к файлу путем объединения пути к папке path и имени текущего файла file.Name().
					if strings.ToLower(filepath.Ext(file.Name())) != ".torrent" {                    //Выделение расширения файла, приведение его к нижнему регистру и проверка. Если это не .torrent, файл игнорируется.
						return                                                                       //Завершение выполнения анонимной функции для текущего файла (переход к следующему элементу в цикле for).
					}

					sp, err := utils.OpenTorrentFile(filename)                                     //Вызов функции парсинга файла: чтение Bencode-структуры, извлечение хеша, трекеров и формирование спецификации TorrentSpec.
					if err != nil {                                                                 //Если файл поврежден, заблокирован или не является валидным торрентом, пишем ошибку в лог и прекращаем обработку этого файла.
						log.TLogln("Error parse file name:", err)                                  
						return
					}

					tor, err := torr.AddTorrent(sp, "", "", "", "")                                      //Добавление распарсенной спецификации торрента в оперативную память активного торрент-движка сервера.
					if err != nil {
						log.TLogln("Error parse torrent file:", err)
						return                                                                  //Если движок не смог принять спецификацию (например, конфликт дескрипторов в памяти), логируем ошибку и выходим из обработки файла.
					}

					if !tor.GotInfo() {                                                          //Проверка, удалось ли торренту получить метаданные (инфо-секцию со списком файлов) из сети или файла.
						log.TLogln("Error get info from torrent")                                 //Если метаданные не получены (например, таймаут или отсутствие пиров), логируем проблему и выходим, оставляя файл в папке для следующей попытки.
						return
					}

					if tor.Title == "" {                                                //Если у торрента отсутствует пользовательское название, в качестве заголовка прописывается оригинальное имя раздачи из метаданных.
						tor.Title = tor.Name()                                           
					}

					// Шаг 4: Безопасное удаление исходного файла торрента до начала записи в БД.
					// Это исключает ситуацию, когда файл считывается повторно в следующем цикле, пока пишется БД.
					if err := os.Remove(filename); err != nil {
						log.TLogln("First delete attempt failed, retrying in 1s...", err)
						time.Sleep(time.Second) // Ожидание разблокировки файла операционной системой
						
						// Вторая (повторная) попытка удаления файла
						if errRetry := os.Remove(filename); errRetry != nil {
							log.TLogln("Second delete attempt failed, skipping DB write:", errRetry)
							return // Прерываем процесс для этого файла во избежание дубликатов в БД
						}
					}

					// Долгая операция с БД теперь безопасна
					torr.SaveTorrentToDB(tor)                                                       //Долгая операция сохранения метаданных торрента в базу данных SQLite. Теперь она выполняется безопасно, так как файл уже гарантированно удален из папки.
					tor.Drop()                                                                       //Выгрузка торрента из активной оперативной памяти сервера (освобождение RAM), так как задача фонового автодобавления завершена.
					time.Sleep(time.Second)                                                          //Короткая пауза в 1 секунду между обработкой отдельных файлов, чтобы снизить пиковую нагрузку на процессор, диск и очереди базы данных.
				}()
			}
		} else {
			log.TLogln("Error read dir:", err)                                                         //Закрытие циклов и ветка else: если саму папку path не удалось прочитать (например, удалена), записываем ошибку в лог сервера.
		}
		time.Sleep(time.Second * 5)                                                                    //
	}
}

///////////
/// DNS
///

// DNSConfig holds DNS resolver configuration
type DNSConfig struct {
	PrimaryServers  []string
	FallbackServers []string
	Timeout         time.Duration
	CacheDuration   time.Duration
}

// DefaultDNSConfig returns a sensible default configuration
func DefaultDNSConfig() DNSConfig {
	return DNSConfig{
		PrimaryServers: []string{
			"8.8.8.8:53", // Google DNS
			"1.1.1.1:53", // CloudFlare DNS
			"9.9.9.9:53", // Quad9 DNS
		},
		FallbackServers: []string{
			"208.67.222.222:53", // OpenDNS
			"64.6.64.6:53",      // Verisign
		},
		Timeout:       5 * time.Second,
		CacheDuration: 5 * time.Minute,
	}
}

// DNSChecker manages DNS resolution with fallback support
type DNSChecker struct {
	config         DNSConfig
	customResolver *net.Resolver
	cache          map[string][]string
	cacheTime      map[string]time.Time
	mu             sync.RWMutex
	useFallback    bool
}

// NewDNSChecker creates a new DNS checker instance
func NewDNSChecker(config DNSConfig) *DNSChecker {
	if len(config.PrimaryServers) == 0 {
		config = DefaultDNSConfig()
	}

	return &DNSChecker{
		config:    config,
		cache:     make(map[string][]string),
		cacheTime: make(map[string]time.Time),
	}
}

// CheckAndResolve performs DNS check and returns a resolver
func (d *DNSChecker) CheckAndResolve() *net.Resolver {
	// Test system DNS first
	if d.testSystemDNS() {
		log.TLogln("System DNS check passed")
		return net.DefaultResolver
	}

	log.TLogln("System DNS check failed, using custom resolver")
	d.initCustomResolver()
	return d.customResolver
}

// testSystemDNS checks if system DNS is working properly
func (d *DNSChecker) testSystemDNS() bool {
	_, cancel := context.WithTimeout(context.Background(), d.config.Timeout)
	defer cancel()

	addrs, err := net.LookupHost("themoviedb.org")
	if err != nil {
		log.TLogln("DNS lookup error:", err)
		return false
	}

	if len(addrs) == 0 {
		log.TLogln("DNS lookup returned no addresses")
		return false
	}

	// Check for suspicious addresses (DNS hijacking/pollution)
	for _, addr := range addrs {
		if isSuspiciousAddress(addr) {
			log.TLogln("Suspicious DNS address detected:", addr)
			return false
		}
	}

	return true
}

// isSuspiciousAddress checks if an address indicates DNS issues
func isSuspiciousAddress(addr string) bool {
	suspiciousPrefixes := []string{
		"127.0.0.1", // Localhost
		"0.0.0.0",   // Invalid address
		"::1",       // IPv6 localhost
		// "10.",       // Private network
		"192.168.", // Private network
		"169.254.", // Link-local
		// "172.16.", "172.17.", "172.18.", "172.19.",
		// "172.20.", "172.21.", "172.22.", "172.23.",
		// "172.24.", "172.25.", "172.26.", "172.27.",
		// "172.28.", "172.29.", "172.30.", "172.31.", // Private network range
	}

	for _, prefix := range suspiciousPrefixes {
		if strings.HasPrefix(addr, prefix) {
			return true
		}
	}

	return false
}

// initCustomResolver creates a custom resolver with fallback support
func (d *DNSChecker) initCustomResolver() {
	d.customResolver = &net.Resolver{
		PreferGo: true, // Use Go's DNS implementation
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   d.config.Timeout,
				KeepAlive: 30 * time.Second,
			}

			// Try primary servers first
			for _, dns := range d.config.PrimaryServers {
				conn, err := dialer.DialContext(ctx, network, dns)
				if err == nil {
					return conn, nil
				}
				log.TLogln("Failed to connect to DNS server", dns, ":", err)
			}

			// Try fallback servers if primary fails
			for _, dns := range d.config.FallbackServers {
				conn, err := dialer.DialContext(ctx, network, dns)
				if err == nil {
					log.TLogln("Using fallback DNS server:", dns)
					return conn, nil
				}
				log.TLogln("Failed to connect to fallback DNS", dns, ":", err)
			}

			return nil, fmt.Errorf("all DNS servers failed")
		},
	}

	d.useFallback = true
}

// LookupHostWithFallback performs DNS lookup with automatic fallback
func (d *DNSChecker) LookupHostWithFallback(host string) ([]string, error) {
	// Check cache first
	if addrs, ok := d.getFromCache(host); ok {
		return addrs, nil
	}

	// Use appropriate resolver
	var resolver *net.Resolver
	if d.useFallback && d.customResolver != nil {
		resolver = d.customResolver
	} else {
		resolver = net.DefaultResolver
	}

	// Perform lookup with timeout
	ctx, cancel := context.WithTimeout(context.Background(), d.config.Timeout)
	defer cancel()

	addrs, err := resolver.LookupHost(ctx, host)
	if err != nil {
		// If using system DNS fails, try custom resolver
		if !d.useFallback && d.customResolver != nil {
			log.TLogln("System DNS failed, trying custom resolver")
			addrs, err = d.customResolver.LookupHost(ctx, host)
		}
	}

	// Cache successful results
	if err == nil && len(addrs) > 0 {
		d.addToCache(host, addrs)
	}

	return addrs, err
}

// getFromCache retrieves DNS results from cache
func (d *DNSChecker) getFromCache(host string) ([]string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if addrs, ok := d.cache[host]; ok {
		if time.Since(d.cacheTime[host]) < d.config.CacheDuration {
			return addrs, true
		}
		// Expired, remove from cache
		delete(d.cache, host)
		delete(d.cacheTime, host)
	}

	return nil, false
}

// addToCache adds DNS results to cache
func (d *DNSChecker) addToCache(host string, addrs []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cache[host] = addrs
	d.cacheTime[host] = time.Now()
}

// Simple usage function (backward compatible)
func dnsResolve() {
	checker := NewDNSChecker(DefaultDNSConfig())
	resolver := checker.CheckAndResolve()

	// Store the resolver for later use if needed
	net.DefaultResolver = resolver // Optional: replace global resolver

	// Test the resolver
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs, err := resolver.LookupHost(ctx, "themoviedb.org")
	if err != nil {
		log.TLogln("DNS resolution test failed:", err)
	} else {
		log.TLogln("DNS resolution successful, addresses:", addrs)
	}
}

// func dnsResolve() {
// 	addrs, err := net.LookupHost("themoviedb.org")
// 	if len(addrs) == 0 {
// 		log.TLogln("System DNS check failed", err)

// 		fn := func(ctx context.Context, network, address string) (net.Conn, error) {
// 			d := net.Dialer{}
// 			return d.DialContext(ctx, "udp", "1.1.1.1:53")
// 		}

// 		net.DefaultResolver = &net.Resolver{
// 			Dial: fn,
// 		}

// 		addrs, err = net.LookupHost("themoviedb.org")
// 		if err != nil {
// 			log.TLogln("Check CloudFlare DNS error:", err)
// 		} else {
// 			log.TLogln("Use CloudFlare DNS")
// 		}
// 	} else {
// 		log.TLogln("System DNS check passed")
// 	}
// }
