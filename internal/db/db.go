package db

import (
    "database/sql"
    "fmt"
    "log"
    "strings"
    
    _ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func Initialize(dsn string) error {
    // Parse DSN to extract database name
    parts := strings.Split(dsn, "/")
    if len(parts) < 2 {
        return fmt.Errorf("invalid DSN format")
    }
    
    dbName := strings.Split(parts[1], "?")[0]
    baseDSN := parts[0] + "/?" + strings.Join(strings.Split(parts[1], "?")[1:], "?")
    
    // First connect without database to create it if needed
    tempDB, err := sql.Open("mysql", baseDSN)
    if err != nil {
        return fmt.Errorf("failed to open connection: %v", err)
    }
    
    // Create database if it doesn't exist
    _, err = tempDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName))
    if err != nil {
        tempDB.Close()
        return fmt.Errorf("failed to create database: %v", err)
    }
    tempDB.Close()
    
    // Now connect to the actual database
    DB, err = sql.Open("mysql", dsn)
    if err != nil {
        return fmt.Errorf("failed to open database: %v", err)
    }
    
    if err = DB.Ping(); err != nil {
        return fmt.Errorf("failed to connect to database: %v", err)
    }
    
    // Create tables if not exist
    if err = createTables(); err != nil {
        return fmt.Errorf("failed to create tables: %v", err)
    }
    
    log.Println("Database initialized successfully")
    return nil
}

func createTables() error {
    queries := []string{
        `CREATE TABLE IF NOT EXISTS providers (
            id INT AUTO_INCREMENT PRIMARY KEY,
            name VARCHAR(100) UNIQUE NOT NULL,
            type ENUM('inbound', 'intermediate', 'final') NOT NULL,
            host VARCHAR(255) NOT NULL,
            port INT DEFAULT 5060,
            username VARCHAR(100),
            password VARCHAR(100),
            auth_type ENUM('ip', 'credentials', 'both') DEFAULT 'credentials',
            codecs JSON,
            max_channels INT DEFAULT 0,
            priority INT DEFAULT 0,
            weight INT DEFAULT 1,
            active BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX idx_type (type),
            INDEX idx_active (active)
        )`,
        
        `CREATE TABLE IF NOT EXISTS dids (
            id INT AUTO_INCREMENT PRIMARY KEY,
            number VARCHAR(20) UNIQUE NOT NULL,
            provider_id INT,
            provider_name VARCHAR(100),
            in_use BOOLEAN DEFAULT FALSE,
            destination VARCHAR(20),
            country VARCHAR(50),
            city VARCHAR(50),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX idx_in_use (in_use),
            INDEX idx_provider (provider_name),
            FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE SET NULL
        )`,
        
        `CREATE TABLE IF NOT EXISTS provider_routes (
            id INT AUTO_INCREMENT PRIMARY KEY,
            name VARCHAR(100) UNIQUE NOT NULL,
            inbound_provider VARCHAR(100) NOT NULL,
            intermediate_provider VARCHAR(100) NOT NULL,
            final_provider VARCHAR(100) NOT NULL,
            load_balance_mode ENUM('round_robin', 'weighted', 'priority', 'failover') DEFAULT 'round_robin',
            priority INT DEFAULT 0,
            active BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            INDEX idx_inbound (inbound_provider),
            INDEX idx_active (active)
        )`,
        
        `CREATE TABLE IF NOT EXISTS call_records (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            call_id VARCHAR(100) UNIQUE NOT NULL,
            original_ani VARCHAR(20) NOT NULL,
            original_dnis VARCHAR(20) NOT NULL,
            transformed_ani VARCHAR(20),
            assigned_did VARCHAR(20),
            inbound_provider VARCHAR(100),
            intermediate_provider VARCHAR(100),
            final_provider VARCHAR(100),
            status VARCHAR(20) DEFAULT 'ACTIVE',
            current_step VARCHAR(20),
            start_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            end_time TIMESTAMP NULL,
            duration INT DEFAULT 0,
            recording_path VARCHAR(255),
            INDEX idx_call_id (call_id),
            INDEX idx_did (assigned_did),
            INDEX idx_status (status),
            INDEX idx_start_time (start_time)
        )`,
        
        `CREATE TABLE IF NOT EXISTS provider_stats (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            provider_name VARCHAR(100) NOT NULL,
            total_calls BIGINT DEFAULT 0,
            active_calls INT DEFAULT 0,
            failed_calls BIGINT DEFAULT 0,
            success_rate DECIMAL(5,2) DEFAULT 0,
            avg_call_duration DECIMAL(10,2) DEFAULT 0,
            last_call_time TIMESTAMP NULL,
            is_healthy BOOLEAN DEFAULT TRUE,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            UNIQUE KEY unique_provider (provider_name),
            INDEX idx_provider (provider_name)
        )`,
        
        `CREATE TABLE IF NOT EXISTS call_verifications (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            call_id VARCHAR(100) NOT NULL,
            verification_step VARCHAR(20) NOT NULL,
            expected_ani VARCHAR(20),
            expected_dnis VARCHAR(20),
            received_ani VARCHAR(20),
            received_dnis VARCHAR(20),
            source_ip VARCHAR(45),
            verified BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            INDEX idx_call_id (call_id)
        )`,
    }
    
    for _, query := range queries {
        if _, err := DB.Exec(query); err != nil {
            return err
        }
    }
    
    return nil
}

func Close() {
    if DB != nil {
        DB.Close()
    }
}
