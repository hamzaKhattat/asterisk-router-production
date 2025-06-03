package ara

import (
    "database/sql"
//    "encoding/json"
    "fmt"
    "log"
    "strings"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
    "github.com/hamzaKhattat/asterisk-router-production/internal/models"
)

// ARA Manager handles all Asterisk Realtime Architecture operations
type Manager struct {
    db *sql.DB
}

func NewManager() *Manager {
    return &Manager{
        db: db.DB,
    }
}

// CreateARATablesIfNotExist creates all necessary ARA tables
func (m *Manager) CreateARATablesIfNotExist() error {
    queries := []string{
        // ps_endpoints table for PJSIP endpoints
        `CREATE TABLE IF NOT EXISTS ps_endpoints (
            id VARCHAR(100) PRIMARY KEY,
            transport VARCHAR(40),
            aors VARCHAR(200),
            auth VARCHAR(100),
            context VARCHAR(40) DEFAULT 'router-context',
            disallow VARCHAR(200) DEFAULT 'all',
            allow VARCHAR(200),
            direct_media ENUM('yes','no') DEFAULT 'no',
            dtmf_mode ENUM('rfc4733','inband','info','auto') DEFAULT 'rfc4733',
            language VARCHAR(10) DEFAULT 'en',
            rtp_timeout int DEFAULT 120,
            force_rport ENUM('yes','no') DEFAULT 'yes',
            rewrite_contact ENUM('yes','no') DEFAULT 'yes',
            trust_id_inbound ENUM('yes','no') DEFAULT 'yes',
            trust_id_outbound ENUM('yes','no') DEFAULT 'yes',
            send_pai ENUM('yes','no') DEFAULT 'yes',
            send_rpid ENUM('yes','no') DEFAULT 'yes',
            record_on_feature VARCHAR(40) DEFAULT 'automixmon',
            record_off_feature VARCHAR(40) DEFAULT 'automixmon'
        )`,
        
        // ps_auths table for authentication
        `CREATE TABLE IF NOT EXISTS ps_auths (
            id VARCHAR(100) PRIMARY KEY,
            auth_type ENUM('userpass','md5') DEFAULT 'userpass',
            username VARCHAR(100),
            password VARCHAR(100),
            realm VARCHAR(100),
            md5_cred VARCHAR(100)
        )`,
        
        // ps_aors table for address of record
        `CREATE TABLE IF NOT EXISTS ps_aors (
            id VARCHAR(100) PRIMARY KEY,
            max_contacts INT DEFAULT 1,
            remove_existing ENUM('yes','no') DEFAULT 'yes',
            contact VARCHAR(255),
            qualify_frequency INT DEFAULT 60,
            authenticate_qualify ENUM('yes','no') DEFAULT 'no'
        )`,
        
        // ps_endpoint_id_ips for IP-based authentication
        // Fixed: 'match' is a reserved keyword, using backticks
        `CREATE TABLE IF NOT EXISTS ps_endpoint_id_ips (
            id VARCHAR(100) PRIMARY KEY,
            endpoint VARCHAR(100),
            ` + "`match`" + ` VARCHAR(100),
            srv_lookups ENUM('yes','no') DEFAULT 'no',
            match_header VARCHAR(255)
        )`,
        
        // ps_transports table
        `CREATE TABLE IF NOT EXISTS ps_transports (
            id VARCHAR(100) PRIMARY KEY,
            async_operations INT DEFAULT 1,
            bind VARCHAR(100) DEFAULT '0.0.0.0:5060',
            protocol ENUM('udp','tcp','tls','ws','wss') DEFAULT 'udp',
            tos VARCHAR(10) DEFAULT 'cs0',
            cos INT DEFAULT 0,
            allow_reload ENUM('yes','no') DEFAULT 'yes'
        )`,
        
        // extensions table for dialplan
        `CREATE TABLE IF NOT EXISTS extensions (
            id INT AUTO_INCREMENT PRIMARY KEY,
            context VARCHAR(40) NOT NULL,
            exten VARCHAR(40) NOT NULL,
            priority INT NOT NULL,
            app VARCHAR(40) NOT NULL,
            appdata VARCHAR(256),
            UNIQUE KEY context_exten_priority (context, exten, priority)
        )`,
        
        // ps_globals table
        `CREATE TABLE IF NOT EXISTS ps_globals (
            id VARCHAR(100) PRIMARY KEY,
            max_forwards INT DEFAULT 70,
            keep_alive_interval INT DEFAULT 30,
            contact_expiration_check_interval INT DEFAULT 30,
            disable_multi_domain ENUM('yes','no') DEFAULT 'no',
            max_initial_qualify_time INT DEFAULT 0,
            unidentified_request_period INT DEFAULT 5,
            unidentified_request_count INT DEFAULT 5,
            default_from_user VARCHAR(80) DEFAULT 'asterisk',
            default_realm VARCHAR(80) DEFAULT 'asterisk'
        )`,
    }
    
    for _, query := range queries {
        if _, err := m.db.Exec(query); err != nil {
            return fmt.Errorf("failed to create ARA table: %v", err)
        }
    }
    
    // Insert default transport if not exists
    m.db.Exec(`INSERT IGNORE INTO ps_transports (id, bind, protocol) VALUES ('transport-udp', '0.0.0.0:5060', 'udp')`)
    
    // Insert global settings
    m.db.Exec(`INSERT IGNORE INTO ps_globals (id) VALUES ('global')`)
    
    return nil
}

// CreateEndpoint creates a complete PJSIP endpoint with ARA
func (m *Manager) CreateEndpoint(provider *models.Provider) error {
    endpointID := fmt.Sprintf("endpoint-%s", provider.Name)
    authID := fmt.Sprintf("auth-%s", provider.Name)
    aorID := fmt.Sprintf("aor-%s", provider.Name)
    
    // Create AOR
    aorQuery := `
        INSERT INTO ps_aors (id, max_contacts, remove_existing, qualify_frequency)
        VALUES (?, 1, 'yes', 60)
        ON DUPLICATE KEY UPDATE
            max_contacts = VALUES(max_contacts),
            qualify_frequency = VALUES(qualify_frequency)`
    
    if _, err := m.db.Exec(aorQuery, aorID); err != nil {
        return err
    }
    
    // Create Auth based on auth type
    if provider.AuthType == "credentials" || provider.AuthType == "both" {
        authQuery := `
            INSERT INTO ps_auths (id, auth_type, username, password)
            VALUES (?, 'userpass', ?, ?)
            ON DUPLICATE KEY UPDATE
                username = VALUES(username),
                password = VALUES(password)`
        
        if _, err := m.db.Exec(authQuery, authID, provider.Username, provider.Password); err != nil {
            return err
        }
    }
    
    // Create Endpoint
    codecs := strings.Join(provider.Codecs, ",")
    if codecs == "" {
        codecs = "ulaw,alaw"
    }
    
    context := "from-provider-" + provider.Type
    
    endpointQuery := `
        INSERT INTO ps_endpoints (
            id, transport, aors, auth, context, 
            disallow, allow, direct_media, trust_id_inbound, trust_id_outbound
        ) VALUES (?, 'transport-udp', ?, ?, ?, 'all', ?, 'no', 'yes', 'yes')
        ON DUPLICATE KEY UPDATE
            transport = VALUES(transport),
            aors = VALUES(aors),
            auth = VALUES(auth),
            context = VALUES(context),
            allow = VALUES(allow)`
    
    authRef := ""
    if provider.AuthType == "credentials" || provider.AuthType == "both" {
        authRef = authID
    }
    
    if _, err := m.db.Exec(endpointQuery, endpointID, aorID, authRef, context, codecs); err != nil {
        return err
    }
    
    // Create IP-based authentication if needed
    if provider.AuthType == "ip" || provider.AuthType == "both" {
        // Using backticks for the match column
        ipQuery := `
            INSERT INTO ps_endpoint_id_ips (id, endpoint, ` + "`match`" + `)
            VALUES (?, ?, ?)
            ON DUPLICATE KEY UPDATE
                endpoint = VALUES(endpoint),
                ` + "`match`" + ` = VALUES(` + "`match`" + `)`
        
        ipID := fmt.Sprintf("ip-%s", provider.Name)
        match := fmt.Sprintf("%s/32", provider.Host)
        
        if _, err := m.db.Exec(ipQuery, ipID, endpointID, match); err != nil {
            return err
        }
    }
    
    log.Printf("Created ARA endpoint for provider %s (auth: %s)", provider.Name, provider.AuthType)
    return nil
}

// DeleteEndpoint removes a PJSIP endpoint from ARA
func (m *Manager) DeleteEndpoint(providerName string) error {
    endpointID := fmt.Sprintf("endpoint-%s", providerName)
    authID := fmt.Sprintf("auth-%s", providerName)
    aorID := fmt.Sprintf("aor-%s", providerName)
    ipID := fmt.Sprintf("ip-%s", providerName)
    
    // Delete in reverse order of creation
    m.db.Exec("DELETE FROM ps_endpoint_id_ips WHERE id = ?", ipID)
    m.db.Exec("DELETE FROM ps_endpoints WHERE id = ?", endpointID)
    m.db.Exec("DELETE FROM ps_auths WHERE id = ?", authID)
    m.db.Exec("DELETE FROM ps_aors WHERE id = ?", aorID)
    
    return nil
}

// CreateDialplan creates the complete dialplan in ARA
func (m *Manager) CreateDialplan() error {
    // Clear existing dialplan for our contexts
    contexts := []string{
        "from-provider-inbound",
        "from-provider-intermediate", 
        "from-provider-final",
        "router-outbound",
    }
    
    for _, ctx := range contexts {
        m.db.Exec("DELETE FROM extensions WHERE context = ?", ctx)
    }
    
    // Create inbound context (from S1)
    inboundExtensions := []struct {
        exten    string
        priority int
        app      string
        appdata  string
    }{
        {"_X.", 1, "NoOp", "Incoming call from S1: ${CALLERID(num)} -> ${EXTEN}"},
        {"_X.", 2, "Set", "CHANNEL(hangup_handler_push)=hangup-handler,s,1"},
        {"_X.", 3, "Set", "__CALLID=${UNIQUEID}"},
        {"_X.", 4, "Set", "__INBOUND_PROVIDER=${CHANNEL(endpoint)}"},
        {"_X.", 5, "Set", "__ORIGINAL_ANI=${CALLERID(num)}"},
        {"_X.", 6, "Set", "__ORIGINAL_DNIS=${EXTEN}"},
        {"_X.", 7, "MixMonitor", "${UNIQUEID}.wav,b"},
        {"_X.", 8, "AGI", "agi://localhost:8002/processIncoming"},
        {"_X.", 9, "GotoIf", "$[\"${ROUTER_STATUS}\" = \"success\"]?10:99"},
        {"_X.", 10, "Set", "CALLERID(num)=${ANI_TO_SEND}"},
        {"_X.", 11, "Dial", "PJSIP/${DNIS_TO_SEND}@${NEXT_HOP},180,U(subrecord^${UNIQUEID})"},
        {"_X.", 12, "Goto", "99"},
        {"_X.", 99, "Congestion", "5"},
        {"_X.", 100, "Hangup", ""},
    }
    
    for _, ext := range inboundExtensions {
        m.insertExtension("from-provider-inbound", ext.exten, ext.priority, ext.app, ext.appdata)
    }
    
    // Create intermediate context (from S3)
    intermediateExtensions := []struct {
        exten    string
        priority int
        app      string
        appdata  string
    }{
        {"_X.", 1, "NoOp", "Return call from S3: ${CALLERID(num)} -> ${EXTEN}"},
        {"_X.", 2, "Set", "__INTERMEDIATE_PROVIDER=${CHANNEL(endpoint)}"},
        {"_X.", 3, "Set", "__SOURCE_IP=${CHANNEL(pjsip,remote_addr)}"},
        {"_X.", 4, "AGI", "agi://localhost:8002/processReturn"},
        {"_X.", 5, "GotoIf", "$[\"${ROUTER_STATUS}\" = \"success\"]?6:99"},
        {"_X.", 6, "Set", "CALLERID(num)=${ANI_TO_SEND}"},
        {"_X.", 7, "Dial", "PJSIP/${DNIS_TO_SEND}@${NEXT_HOP},180"},
        {"_X.", 8, "Goto", "99"},
        {"_X.", 99, "Congestion", "5"},
        {"_X.", 100, "Hangup", ""},
    }
    
    for _, ext := range intermediateExtensions {
        m.insertExtension("from-provider-intermediate", ext.exten, ext.priority, ext.app, ext.appdata)
    }
    
    // Create final context (from S4)
    finalExtensions := []struct {
        exten    string
        priority int
        app      string
        appdata  string
    }{
        {"_X.", 1, "NoOp", "Final call from S4: ${CALLERID(num)} -> ${EXTEN}"},
        {"_X.", 2, "Set", "__FINAL_PROVIDER=${CHANNEL(endpoint)}"},
        {"_X.", 3, "Set", "__SOURCE_IP=${CHANNEL(pjsip,remote_addr)}"},
        {"_X.", 4, "AGI", "agi://localhost:8002/processFinal"},
        {"_X.", 5, "Congestion", "5"},
        {"_X.", 6, "Hangup", ""},
    }
    
    for _, ext := range finalExtensions {
        m.insertExtension("from-provider-final", ext.exten, ext.priority, ext.app, ext.appdata)
    }
    
    // Create hangup handler
    hangupExtensions := []struct {
        exten    string
        priority int
        app      string
        appdata  string
    }{
        {"s", 1, "NoOp", "Call ended: ${UNIQUEID}"},
        {"s", 2, "AGI", "agi://localhost:8002/hangup"},
        {"s", 3, "Return", ""},
    }
    
    for _, ext := range hangupExtensions {
        m.insertExtension("hangup-handler", ext.exten, ext.priority, ext.app, ext.appdata)
    }
    
    // Create subroutine for recording on originated calls
    subrecordExtensions := []struct {
        exten    string
        priority int
        app      string
        appdata  string
    }{
        {"s", 1, "NoOp", "Starting recording on originated channel"},
        {"s", 2, "MixMonitor", "${ARG1}-out.wav,b"},
        {"s", 3, "Return", ""},
    }
    
    for _, ext := range subrecordExtensions {
        m.insertExtension("subrecord", ext.exten, ext.priority, ext.app, ext.appdata)
    }
    
    log.Println("Dialplan created successfully in ARA")
    return nil
}

func (m *Manager) insertExtension(context, exten string, priority int, app, appdata string) error {
    query := `
        INSERT INTO extensions (context, exten, priority, app, appdata)
        VALUES (?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
            app = VALUES(app),
            appdata = VALUES(appdata)`
    
    _, err := m.db.Exec(query, context, exten, priority, app, appdata)
    return err
}

// ReloadDialplan triggers Asterisk to reload dialplan from ARA
func (m *Manager) ReloadDialplan() error {
    // This would use AMI to trigger reload
    // For now, log the action
    log.Println("Dialplan reload triggered")
    return nil
}
