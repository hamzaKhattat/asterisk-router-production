package provider

import (
//    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "sync"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/ara"
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
    "github.com/hamzaKhattat/asterisk-router-production/internal/models"
)

type Manager struct {
    mu             sync.RWMutex
    providers      map[string]*models.Provider
    providerRoutes map[string]*models.ProviderRoute
    araManager     *ara.Manager
}

func NewManager() *Manager {
    return &Manager{
        providers:      make(map[string]*models.Provider),
        providerRoutes: make(map[string]*models.ProviderRoute),
        araManager:     ara.NewManager(),
    }
}

func (m *Manager) Initialize() error {
    // Create ARA tables
    if err := m.araManager.CreateARATablesIfNotExist(); err != nil {
        return fmt.Errorf("failed to create ARA tables: %v", err)
    }
    
    // Load providers from database
    if err := m.LoadProviders(); err != nil {
        return err
    }
    
    // Load routes
    if err := m.LoadRoutes(); err != nil {
        return err
    }
    
    // Create dialplan
    if err := m.araManager.CreateDialplan(); err != nil {
        return err
    }
    
    return nil
}

func (m *Manager) AddProvider(p *models.Provider) error {
    // Validate
    if p.Name == "" || p.Host == "" {
        return fmt.Errorf("provider name and host are required")
    }
    
    if p.Port == 0 {
        p.Port = 5060
    }
    
    // Determine auth type based on username/password
    if p.Username == "" && p.Password == "" {
        p.AuthType = "ip"
    } else {
        p.AuthType = "credentials"
    }
    
    // Default codecs if none specified
    if len(p.Codecs) == 0 {
        p.Codecs = []string{"ulaw", "alaw"}
    }
    
    // Store in database
    codecsJSON, _ := json.Marshal(p.Codecs)
    
    query := `
        INSERT INTO providers (name, type, host, port, username, password, auth_type, codecs, max_channels, priority, weight, active)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
            type = VALUES(type),
            host = VALUES(host),
            port = VALUES(port),
            username = VALUES(username),
            password = VALUES(password),
            auth_type = VALUES(auth_type),
            codecs = VALUES(codecs),
            max_channels = VALUES(max_channels),
            priority = VALUES(priority),
            weight = VALUES(weight),
            active = VALUES(active)`
    
    result, err := db.DB.Exec(query, p.Name, p.Type, p.Host, p.Port, p.Username, p.Password, p.AuthType, codecsJSON, p.MaxChannels, p.Priority, p.Weight, p.Active)
    if err != nil {
        return err
    }
    
    if p.ID == 0 {
        id, _ := result.LastInsertId()
        p.ID = int(id)
    }
    
    // Store in memory
    m.mu.Lock()
    m.providers[p.Name] = p
    m.mu.Unlock()
    
    // Create ARA endpoint
    if err := m.araManager.CreateEndpoint(p); err != nil {
        return fmt.Errorf("failed to create ARA endpoint: %v", err)
    }
    
    log.Printf("Provider %s added successfully", p.Name)
    return nil
}

func (m *Manager) GetProvider(name string) (*models.Provider, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    provider, exists := m.providers[name]
    if !exists {
        return nil, fmt.Errorf("provider %s not found", name)
    }
    
    return provider, nil
}

func (m *Manager) GetProvidersByName(namePattern string) ([]*models.Provider, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    var providers []*models.Provider
    
    // Check if it's an exact match first
    if p, exists := m.providers[namePattern]; exists {
        providers = append(providers, p)
        return providers, nil
    }
    
    // Otherwise, look for pattern match (e.g., for load balancing groups)
    for name, p := range m.providers {
        if name == namePattern || p.Type == namePattern {
            providers = append(providers, p)
        }
    }
    
    if len(providers) == 0 {
        return nil, fmt.Errorf("no providers found matching %s", namePattern)
    }
    
    return providers, nil
}

func (m *Manager) ListProviders(providerType string) ([]*models.Provider, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    var providers []*models.Provider
    for _, p := range m.providers {
        if providerType == "" || p.Type == providerType {
            providers = append(providers, p)
        }
    }
    
    return providers, nil
}

func (m *Manager) DeleteProvider(name string) error {
    // Check if provider is used in any routes
    var count int
    db.DB.QueryRow("SELECT COUNT(*) FROM provider_routes WHERE inbound_provider = ? OR intermediate_provider = ? OR final_provider = ?", 
        name, name, name).Scan(&count)
    if count > 0 {
        return fmt.Errorf("provider %s is used in %d routes", name, count)
    }
    
    // Delete from ARA
    if err := m.araManager.DeleteEndpoint(name); err != nil {
        log.Printf("Failed to delete ARA endpoint: %v", err)
    }
    
    // Delete from database
    if _, err := db.DB.Exec("DELETE FROM providers WHERE name = ?", name); err != nil {
        return err
    }
    
    // Remove from memory
    m.mu.Lock()
    delete(m.providers, name)
    m.mu.Unlock()
    
    return nil
}

func (m *Manager) LoadProviders() error {
    query := `
        SELECT id, name, type, host, port, username, password, auth_type, codecs, max_channels, priority, weight, active
        FROM providers
        WHERE active = TRUE`
    
    rows, err := db.DB.Query(query)
    if err != nil {
        return err
    }
    defer rows.Close()
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.providers = make(map[string]*models.Provider)
    
    for rows.Next() {
        p := &models.Provider{}
        var codecsJSON []byte
        
        err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Host, &p.Port, &p.Username, &p.Password, &p.AuthType, &codecsJSON, &p.MaxChannels, &p.Priority, &p.Weight, &p.Active)
        if err != nil {
            log.Printf("Error loading provider: %v", err)
            continue
        }
        
        json.Unmarshal(codecsJSON, &p.Codecs)
        m.providers[p.Name] = p
        
        // Create ARA endpoint
        m.araManager.CreateEndpoint(p)
    }
    
    log.Printf("Loaded %d providers", len(m.providers))
    return nil
}

// Route management
func (m *Manager) AddProviderRoute(route *models.ProviderRoute) error {
    // Validate providers exist
    for _, providerName := range []string{route.InboundProvider, route.IntermediateProvider, route.FinalProvider} {
        if _, err := m.GetProvider(providerName); err != nil {
            return fmt.Errorf("provider %s not found", providerName)
        }
    }
    
    query := `
        INSERT INTO provider_routes (name, inbound_provider, intermediate_provider, final_provider, load_balance_mode, priority, active)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
            inbound_provider = VALUES(inbound_provider),
            intermediate_provider = VALUES(intermediate_provider),
            final_provider = VALUES(final_provider),
            load_balance_mode = VALUES(load_balance_mode),
            priority = VALUES(priority),
            active = VALUES(active)`
    
    result, err := db.DB.Exec(query, route.Name, route.InboundProvider, route.IntermediateProvider, route.FinalProvider, route.LoadBalanceMode, route.Priority, route.Active)
    if err != nil {
        return err
    }
    
    if route.ID == 0 {
        id, _ := result.LastInsertId()
        route.ID = int(id)
    }
    
    m.mu.Lock()
    m.providerRoutes[route.Name] = route
    m.mu.Unlock()
    
    log.Printf("Provider route %s created: %s -> %s -> %s", route.Name, route.InboundProvider, route.IntermediateProvider, route.FinalProvider)
    return nil
}

func (m *Manager) GetRouteForInbound(inboundProvider string) (*models.ProviderRoute, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    // Find route with highest priority for this inbound provider
    var bestRoute *models.ProviderRoute
    highestPriority := -1
    
    for _, route := range m.providerRoutes {
        if route.InboundProvider == inboundProvider && route.Active && route.Priority > highestPriority {
            bestRoute = route
            highestPriority = route.Priority
        }
    }
    
    if bestRoute == nil {
        return nil, fmt.Errorf("no active route found for inbound provider %s", inboundProvider)
    }
    
    return bestRoute, nil
}

func (m *Manager) LoadRoutes() error {
    query := `
        SELECT id, name, inbound_provider, intermediate_provider, final_provider, load_balance_mode, priority, active
        FROM provider_routes
        WHERE active = TRUE`
    
    rows, err := db.DB.Query(query)
    if err != nil {
        // Table might not exist yet
        return nil
    }
    defer rows.Close()
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.providerRoutes = make(map[string]*models.ProviderRoute)
    
    for rows.Next() {
        route := &models.ProviderRoute{}
        err := rows.Scan(&route.ID, &route.Name, &route.InboundProvider, &route.IntermediateProvider, &route.FinalProvider, &route.LoadBalanceMode, &route.Priority, &route.Active)
        if err != nil {
            log.Printf("Error loading route: %v", err)
            continue
        }
        
        m.providerRoutes[route.Name] = route
    }
    
    log.Printf("Loaded %d routes", len(m.providerRoutes))
    return nil
}

// GetRouterStats returns router statistics
func (m *Manager) GetRouterStats() map[string]interface{} {
    stats := make(map[string]interface{})
    
    // Get active calls count (this would typically come from a router instance)
    stats["active_calls"] = 0
    
    // Get DID statistics
    var totalDIDs, usedDIDs, availableDIDs int
    err := db.DB.QueryRow(`
        SELECT 
            COUNT(*) as total,
            SUM(CASE WHEN in_use = 1 THEN 1 ELSE 0 END) as used,
            SUM(CASE WHEN in_use = 0 THEN 1 ELSE 0 END) as available
        FROM dids
    `).Scan(&totalDIDs, &usedDIDs, &availableDIDs)
    
    if err != nil {
        log.Printf("Error getting DID stats: %v", err)
        totalDIDs, usedDIDs, availableDIDs = 0, 0, 0
    }
    
    stats["total_dids"] = totalDIDs
    stats["used_dids"] = usedDIDs
    stats["available_dids"] = availableDIDs
    
    // Get provider count by type
    m.mu.RLock()
    inboundCount := 0
    intermediateCount := 0
    finalCount := 0
    
    for _, p := range m.providers {
        switch p.Type {
        case "inbound":
            inboundCount++
        case "intermediate":
            intermediateCount++
        case "final":
            finalCount++
        }
    }
    m.mu.RUnlock()
    
    stats["inbound_providers"] = inboundCount
    stats["intermediate_providers"] = intermediateCount
    stats["final_providers"] = finalCount
    stats["total_providers"] = len(m.providers)
    stats["total_routes"] = len(m.providerRoutes)
    
    return stats
}
