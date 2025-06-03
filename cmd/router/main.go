package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/spf13/viper"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/agi"
    "github.com/hamzaKhattat/asterisk-router-production/internal/ami"
    "github.com/hamzaKhattat/asterisk-router-production/internal/cli"
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
 //   "github.com/hamzaKhattat/asterisk-router-production/internal/loadbalancer"
    "github.com/hamzaKhattat/asterisk-router-production/internal/provider"
    "github.com/hamzaKhattat/asterisk-router-production/internal/router"
)

func main() {
    // Define base flags
    var (
        configFile = flag.String("config", "configs/router.yaml", "Configuration file path")
        initDB     = flag.Bool("init-db", false, "Initialize database")
        runAGI     = flag.Bool("agi", false, "Run AGI server")
        verbose    = flag.Bool("verbose", false, "Enable verbose logging")
        showHelp   = flag.Bool("help", false, "Show help")
        showVersion = flag.Bool("version", false, "Show version")
    )
    
    // Parse flags first to check for init-db or help
    flag.Parse()
    
    if *showHelp {
        showUsage()
        return
    }
    
    if *showVersion {
        fmt.Println("Asterisk Router v1.0.0")
        return
    }
    
    // Setup logging
    if *verbose {
        log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
    } else {
        log.SetFlags(log.Ldate | log.Ltime)
    }
    
    // Load configuration
    viper.SetConfigFile(*configFile)
    viper.SetDefault("database.host", "localhost")
    viper.SetDefault("database.port", 3306)
    viper.SetDefault("database.user", "root")
    viper.SetDefault("database.password", "temppass")
    viper.SetDefault("database.name", "asterisk_router")
    viper.SetDefault("agi.port", 8002)
    viper.SetDefault("ami.host", "localhost")
    viper.SetDefault("ami.port", 5038)
    viper.SetDefault("ami.username", "admin")
    viper.SetDefault("ami.password", "admin")
    
    if err := viper.ReadInConfig(); err != nil {
        if !*initDB {
            log.Printf("Warning: Could not read config file: %v", err)
        }
    }
    
    // Initialize database
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
        viper.GetString("database.user"),
        viper.GetString("database.password"),
        viper.GetString("database.host"),
        viper.GetInt("database.port"),
        viper.GetString("database.name"))
    
    if err := db.Initialize(dsn); err != nil {
        log.Fatalf("Failed to initialize database: %v", err)
    }
    defer db.Close()
    
    // Initialize provider manager
    providerMgr := provider.NewManager()
    if err := providerMgr.Initialize(); err != nil {
        log.Fatalf("Failed to initialize provider manager: %v", err)
    }
    
    // Handle init-db flag
    if *initDB {
        fmt.Println("Database initialized successfully!")
        fmt.Println("\nNext steps:")
        fmt.Println("1. Add providers:")
        fmt.Println("   ./router provider add s1 --type inbound --host 192.168.1.10")
        fmt.Println("2. Add DIDs:")
        fmt.Println("   ./router did add 18001234567 --provider s3-1")
        fmt.Println("3. Create routes:")
        fmt.Println("   ./router route add main-route s1 s3-1 s4-1")
        fmt.Println("4. Start AGI server:")
        fmt.Println("   ./router -agi -verbose")
        return
    }
    
    // Handle AGI server mode
    if *runAGI {
        runAGIServer(providerMgr, *verbose)
        return
    }
    
    // If no AGI flag, run CLI mode
    runCLI(providerMgr)
}

func runAGIServer(providerMgr *provider.Manager, verbose bool) {
    // Create router
    r := router.NewRouter(providerMgr)
    
    // Start load balancer health monitor
    r.GetLoadBalancer().StartHealthMonitor()
    
    // Create and start AGI server
    agiServer := agi.NewServer(r, viper.GetInt("agi.port"))
    
    go func() {
        log.Printf("Starting AGI server on port %d...", viper.GetInt("agi.port"))
        if err := agiServer.Start(); err != nil {
            log.Fatalf("Failed to start AGI server: %v", err)
        }
    }()
    
    // Connect to AMI if configured
    if viper.GetString("ami.username") != "" {
        amiManager := ami.NewManager(
            viper.GetString("ami.host"),
            viper.GetInt("ami.port"),
            viper.GetString("ami.username"),
            viper.GetString("ami.password"),
        )
        
        if err := amiManager.Connect(); err != nil {
            log.Printf("Warning: Failed to connect to AMI: %v", err)
        } else {
            defer amiManager.Close()
            
            // Monitor AMI events
            go func() {
                for event := range amiManager.Events() {
                    if verbose {
                        log.Printf("AMI Event: %s", event["Event"])
                    }
                }
            }()
        }
    }
    
    fmt.Println("AGI Server running. Press Ctrl+C to stop.")
    
    // Wait for interrupt signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
    agiServer.Stop()
}

func runCLI(providerMgr *provider.Manager) {
    rootCmd := cli.InitCLI(providerMgr)
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func showUsage() {
    fmt.Println(`Asterisk Router Production System

USAGE:
    router [flags] <command> [arguments]

FLAGS:
    -agi            Run AGI server
    -init-db        Initialize database
    -verbose        Enable verbose logging
    -config <file>  Configuration file (default: configs/router.yaml)
    -help           Show this help
    -version        Show version

COMMANDS:
    provider        Manage providers
    did             Manage DIDs
    route           Manage routes
    stats           Show system statistics
    lb              Show load balancer status
    calls           Show active calls
    monitor         Monitor system in real-time

EXAMPLES:
    # Initialize database
    ./router -init-db

    # Add a provider
    ./router provider add s1 --type inbound --host 192.168.1.10

    # Add DIDs
    ./router did add 18001234567 18001234568 --provider s3-1
    ./router did add --file dids.csv --provider s3-1

    # Create a route
    ./router route add main-route s1 s3-1 s4-1 --mode round_robin

    # List providers
    ./router provider list
    ./router provider list --type intermediate

    # Show statistics
    ./router stats
    ./router stats --providers

    # Run AGI server
    ./router -agi -verbose

For more information on a command, use:
    ./router <command> --help`)
}
