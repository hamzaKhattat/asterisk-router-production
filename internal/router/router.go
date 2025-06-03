package router

import (
    "database/sql"
    "fmt"
    "log"
    "strings"
    "sync"
    "time"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
    "github.com/hamzaKhattat/asterisk-router-production/internal/loadbalancer"
    "github.com/hamzaKhattat/asterisk-router-production/internal/models"
    "github.com/hamzaKhattat/asterisk-router-production/internal/provider"
)

type Router struct {
    providerMgr  *provider.Manager
    loadBalancer *loadbalancer.LoadBalancer
    mu           sync.RWMutex
    activeCalls  map[string]*models.CallRecord
    didToCall    map[string]string // DID -> CallID mapping
}

func NewRouter(providerMgr *provider.Manager) *Router {
    r := &Router{
        providerMgr:  providerMgr,
        loadBalancer: loadbalancer.New(),
        activeCalls:  make(map[string]*models.CallRecord),
        didToCall:    make(map[string]string),
    }
    
    // Start load balancer health monitor
    r.loadBalancer.StartHealthMonitor()
    
    go r.cleanupRoutine()
    return r
}

// ProcessIncomingCall handles call from S1 to S2 (Step 1 in UML)
func (r *Router) ProcessIncomingCall(callID, ani, dnis, inboundProvider string) (*models.CallResponse, error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    log.Printf("[ROUTER] ========== INCOMING CALL ==========")
    log.Printf("[ROUTER] CallID: %s", callID)
    log.Printf("[ROUTER] ANI-1: %s", ani)
    log.Printf("[ROUTER] DNIS-1: %s", dnis)
    log.Printf("[ROUTER] Inbound Provider: %s", inboundProvider)
    log.Printf("[ROUTER] ===================================")
    
    // Get route for this inbound provider
    route, err := r.providerMgr.GetRouteForInbound(inboundProvider)
    if err != nil {
        return nil, fmt.Errorf("no route for inbound provider %s: %v", inboundProvider, err)
    }
    
    log.Printf("[ROUTER] Using route: %s", route.Name)
    
    // Select intermediate provider using load balancing
    intermediateProviders, err := r.providerMgr.GetProvidersByName(route.IntermediateProvider)
    if err != nil {
        return nil, err
    }
    
    intermediateProvider, err := r.loadBalancer.SelectProvider(intermediateProviders, route.LoadBalanceMode)
    if err != nil {
        return nil, err
    }
    
    log.Printf("[ROUTER] Selected intermediate provider: %s (Mode: %s)", intermediateProvider.Name, route.LoadBalanceMode)
    
    // Select final provider
    finalProviders, err := r.providerMgr.GetProvidersByName(route.FinalProvider)
    if err != nil {
        return nil, err
    }
    
    finalProvider, err := r.loadBalancer.SelectProvider(finalProviders, route.LoadBalanceMode)
    if err != nil {
        return nil, err
    }
    
    log.Printf("[ROUTER] Selected final provider: %s", finalProvider.Name)
    
    // Get available DID for the intermediate provider
    did, err := r.getAvailableDID(intermediateProvider.Name)
    if err != nil {
        return nil, fmt.Errorf("no available DIDs for provider %s: %v", intermediateProvider.Name, err)
    }
    
    log.Printf("[ROUTER] Assigned DID: %s", did)
    
    // Mark DID as in use with destination DNIS-1
    if err := r.markDIDInUse(did, dnis); err != nil {
        return nil, err
    }
    
    // Create call record
    record := &models.CallRecord{
        CallID:               callID,
        OriginalANI:          ani,
        OriginalDNIS:         dnis,
        TransformedANI:       dnis,  // ANI-2 = DNIS-1
        AssignedDID:          did,
        InboundProvider:      inboundProvider,
        IntermediateProvider: intermediateProvider.Name,
        FinalProvider:        finalProvider.Name,
        Status:               "ACTIVE",
        CurrentStep:          "S1_TO_S2",
        StartTime:            time.Now(),
        RecordingPath:        fmt.Sprintf("/var/spool/asterisk/monitor/%s.wav", callID),
    }
    
    r.activeCalls[callID] = record
    r.didToCall[did] = callID
    
    // Store in database
    if err := r.storeCallRecord(record); err != nil {
        log.Printf("Failed to store call record: %v", err)
    }
    
    // Store verification record
    r.storeVerificationRecord(callID, "S1_TO_S2", map[string]string{
        "ani": ani,
        "dnis": dnis,
    }, map[string]string{
        "ani": ani,
        "dnis": dnis,
    }, "", true)
    
    // Update load balancer stats
    r.loadBalancer.IncrementActiveCalls(intermediateProvider.Name, 1)
    r.loadBalancer.IncrementActiveCalls(finalProvider.Name, 1)
    
    // Prepare response for S2 to S3 routing
    response := &models.CallResponse{
        Status:      "success",
        DIDAssigned: did,
        NextHop:     fmt.Sprintf("endpoint-%s", intermediateProvider.Name),
        ANIToSend:   dnis,  // ANI-2 = DNIS-1
        DNISToSend:  did,   // DID
    }
    
    log.Printf("[ROUTER] Routing to S3:")
    log.Printf("[ROUTER]   ANI-2: %s (was DNIS-1)", response.ANIToSend)
    log.Printf("[ROUTER]   DID: %s", response.DNISToSend)
    log.Printf("[ROUTER]   Next Hop: %s", response.NextHop)
    
    return response, nil
}

// ProcessReturnCall handles call returning from S3 (Step 3 in UML)
func (r *Router) ProcessReturnCall(ani2, did, provider, sourceIP string) (*models.CallResponse, error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    log.Printf("[ROUTER] ========== RETURN CALL FROM S3 ==========")
    log.Printf("[ROUTER] ANI-2: %s", ani2)
    log.Printf("[ROUTER] DID: %s", did)
    log.Printf("[ROUTER] Provider: %s", provider)
    log.Printf("[ROUTER] Source IP: %s", sourceIP)
    log.Printf("[ROUTER] ========================================")
    
    // Find call by DID
    callID, exists := r.didToCall[did]
    if !exists {
        log.Printf("[ROUTER] ERROR: No active call for DID %s", did)
        return nil, fmt.Errorf("no active call for DID %s", did)
    }
    
    record := r.activeCalls[callID]
    if record == nil {
        log.Printf("[ROUTER] ERROR: No call record for CallID %s", callID)
        return nil, fmt.Errorf("no call record found")
    }
    
    log.Printf("[ROUTER] Found call record: CallID=%s", callID)
    
    // Verify source IP matches intermediate provider
    intermediateProvider, err := r.providerMgr.GetProvider(record.IntermediateProvider)
    if err != nil {
        return nil, fmt.Errorf("intermediate provider not found: %v", err)
    }
    
    if err := r.verifyProviderIP(intermediateProvider, sourceIP); err != nil {
        log.Printf("[ROUTER] ERROR: IP verification failed for %s: %v", record.IntermediateProvider, err)
        r.storeVerificationRecord(callID, "S3_TO_S2", map[string]string{
            "ani": ani2,
            "dnis": did,
            "expected_ip": intermediateProvider.Host,
        }, map[string]string{
            "ani": ani2,
            "dnis": did,
            "actual_ip": sourceIP,
        }, sourceIP, false)
        return nil, fmt.Errorf("unauthorized source IP: %s", sourceIP)
    }
    
    log.Printf("[ROUTER] IP verification passed for %s", record.IntermediateProvider)
    
    // Verify ANI-2 matches original DNIS-1
    if ani2 != record.OriginalDNIS {
        log.Printf("[ROUTER] WARNING: ANI mismatch: expected %s, got %s", record.OriginalDNIS, ani2)
    }
    
    // Store verification record
    r.storeVerificationRecord(callID, "S3_TO_S2", map[string]string{
        "ani": record.OriginalDNIS,
        "dnis": did,
    }, map[string]string{
        "ani": ani2,
        "dnis": did,
    }, sourceIP, true)
    
    // Update call state
    record.CurrentStep = "S3_TO_S2"
    record.Status = "RETURNED_FROM_S3"
    
    // Build response for routing to S4
    response := &models.CallResponse{
        Status:     "success",
        NextHop:    fmt.Sprintf("endpoint-%s", record.FinalProvider),
        ANIToSend:  record.OriginalANI,   // Restore ANI-1
        DNISToSend: record.OriginalDNIS,  // Restore DNIS-1
    }
    
    log.Printf("[ROUTER] Routing to S4:")
    log.Printf("[ROUTER]   ANI-1: %s (restored)", response.ANIToSend)
    log.Printf("[ROUTER]   DNIS-1: %s (restored)", response.DNISToSend)
    log.Printf("[ROUTER]   Next Hop: %s", response.NextHop)
    
    return response, nil
}

// ProcessFinalCall handles the final call from S4 (Step 5 in UML)
func (r *Router) ProcessFinalCall(callID, ani, dnis, provider, sourceIP string) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    log.Printf("[ROUTER] ========== FINAL CALL FROM S4 ==========")
    log.Printf("[ROUTER] CallID: %s", callID)
    log.Printf("[ROUTER] ANI: %s", ani)
    log.Printf("[ROUTER] DNIS: %s", dnis)
    log.Printf("[ROUTER] Provider: %s", provider)
    log.Printf("[ROUTER] Source IP: %s", sourceIP)
    log.Printf("[ROUTER] ========================================")
    
    // Find call record
    record, exists := r.activeCalls[callID]
    if !exists {
        // Try to find by ANI/DNIS combination
        for cid, rec := range r.activeCalls {
            if rec.OriginalANI == ani && rec.OriginalDNIS == dnis {
                record = rec
                callID = cid
                break
            }
        }
        if record == nil {
            log.Printf("[ROUTER] ERROR: Call not found for CallID=%s, ANI=%s, DNIS=%s", callID, ani, dnis)
            return fmt.Errorf("call not found")
        }
    }
    
    log.Printf("[ROUTER] Found call record: CallID=%s", callID)
    
    // Verify source IP matches final provider
    finalProvider, err := r.providerMgr.GetProvider(record.FinalProvider)
    if err != nil {
        return fmt.Errorf("final provider not found: %v", err)
    }
    
    if err := r.verifyProviderIP(finalProvider, sourceIP); err != nil {
        log.Printf("[ROUTER] ERROR: IP verification failed for %s: %v", record.FinalProvider, err)
        r.storeVerificationRecord(callID, "S4_TO_S2", map[string]string{
            "ani": ani,
            "dnis": dnis,
            "expected_ip": finalProvider.Host,
        }, map[string]string{
            "ani": ani,
            "dnis": dnis,
            "actual_ip": sourceIP,
        }, sourceIP, false)
        return fmt.Errorf("unauthorized source IP: %s", sourceIP)
    }
    
    log.Printf("[ROUTER] IP verification passed for %s", record.FinalProvider)
    
    // Verify ANI and DNIS match original values
    if ani != record.OriginalANI || dnis != record.OriginalDNIS {
        log.Printf("[ROUTER] WARNING: Call parameters mismatch")
        log.Printf("[ROUTER]   Expected ANI: %s, Got: %s", record.OriginalANI, ani)
        log.Printf("[ROUTER]   Expected DNIS: %s, Got: %s", record.OriginalDNIS, dnis)
    }
    
    // Store verification record
    r.storeVerificationRecord(callID, "S4_TO_S2", map[string]string{
        "ani": record.OriginalANI,
        "dnis": record.OriginalDNIS,
    }, map[string]string{
        "ani": ani,
        "dnis": dnis,
    }, sourceIP, true)
    
    // Calculate call duration
    duration := time.Since(record.StartTime)
    
    // Update load balancer stats
    r.loadBalancer.UpdateStats(record.IntermediateProvider, true, duration)
    r.loadBalancer.UpdateStats(record.FinalProvider, true, duration)
    r.loadBalancer.IncrementActiveCalls(record.IntermediateProvider, -1)
    r.loadBalancer.IncrementActiveCalls(record.FinalProvider, -1)
    
    // Update call record
    record.Status = "COMPLETED"
    record.CurrentStep = "COMPLETED"
    now := time.Now()
    record.EndTime = &now
    record.Duration = int(duration.Seconds())
    
    // Release DID
    if err := r.releaseDID(record.AssignedDID); err != nil {
        log.Printf("Failed to release DID: %v", err)
    }
    
    log.Printf("[ROUTER] Released DID: %s", record.AssignedDID)
    
    // Update database
    if err := r.updateCallRecord(record); err != nil {
        log.Printf("Failed to update call record: %v", err)
    }
    
    // Clean up
    delete(r.activeCalls, callID)
    delete(r.didToCall, record.AssignedDID)
    
    log.Printf("[ROUTER] Call %s completed successfully (Duration: %v)", callID, duration)
    return nil
}

// Helper functions
func (r *Router) getAvailableDID(providerName string) (string, error) {
    query := `
        SELECT number FROM dids 
        WHERE in_use = 0 AND provider_name = ? 
        ORDER BY RAND() 
        LIMIT 1 
        FOR UPDATE`
    
    var did string
    err := db.DB.QueryRow(query, providerName).Scan(&did)
    if err == sql.ErrNoRows {
        // Try any available DID if provider-specific DID not found
        err = db.DB.QueryRow("SELECT number FROM dids WHERE in_use = 0 ORDER BY RAND() LIMIT 1 FOR UPDATE").Scan(&did)
    }
    
    if err != nil {
        return "", fmt.Errorf("no available DIDs: %v", err)
    }
    
    return did, nil
}

func (r *Router) markDIDInUse(did, destination string) error {
    query := `UPDATE dids SET in_use = 1, destination = ?, updated_at = NOW() WHERE number = ?`
    _, err := db.DB.Exec(query, destination, did)
    return err
}

func (r *Router) releaseDID(did string) error {
    query := `UPDATE dids SET in_use = 0, destination = NULL, updated_at = NOW() WHERE number = ?`
    _, err := db.DB.Exec(query, did)
    return err
}

func (r *Router) verifyProviderIP(provider *models.Provider, sourceIP string) error {
    // Extract IP from source (remove port if present)
    parts := strings.Split(sourceIP, ":")
    ip := parts[0]
    
    // For IP-based auth, verify the source IP
    if provider.AuthType == "ip" || provider.AuthType == "both" {
        if provider.Host != ip {
            return fmt.Errorf("IP mismatch: expected %s, got %s", provider.Host, ip)
        }
    }
    
    return nil
}

func (r *Router) storeCallRecord(record *models.CallRecord) error {
    query := `
        INSERT INTO call_records 
        (call_id, original_ani, original_dnis, transformed_ani, assigned_did, 
         inbound_provider, intermediate_provider, final_provider, status, 
         current_step, start_time, recording_path)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
    
    _, err := db.DB.Exec(query, 
        record.CallID, record.OriginalANI, record.OriginalDNIS,
        record.TransformedANI, record.AssignedDID, record.InboundProvider, 
        record.IntermediateProvider, record.FinalProvider, record.Status, 
        record.CurrentStep, record.StartTime, record.RecordingPath)
    
    return err
}

func (r *Router) updateCallRecord(record *models.CallRecord) error {
    query := `
        UPDATE call_records 
        SET status = ?, current_step = ?, end_time = ?, duration = ?
        WHERE call_id = ?`
    
    _, err := db.DB.Exec(query, record.Status, record.CurrentStep, 
        record.EndTime, record.Duration, record.CallID)
    return err
}

func (r *Router) storeVerificationRecord(callID string, step string, expected, received map[string]string, sourceIP string, verified bool) {
    query := `
        INSERT INTO call_verifications 
        (call_id, verification_step, expected_ani, expected_dnis, 
         received_ani, received_dnis, source_ip, verified)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
    
    _, err := db.DB.Exec(query, callID, step,
        expected["ani"], expected["dnis"],
        received["ani"], received["dnis"],
        sourceIP, verified)
    
    if err != nil {
        log.Printf("Failed to store verification record: %v", err)
    }
}

func (r *Router) cleanupRoutine() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        r.cleanupStaleCalls()
    }
}

func (r *Router) cleanupStaleCalls() {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    now := time.Now()
    for callID, record := range r.activeCalls {
        if now.Sub(record.StartTime) > 30*time.Minute {
            log.Printf("Cleaning up stale call %s", callID)
            
            // Release DID
            r.releaseDID(record.AssignedDID)
            
            // Update stats
            r.loadBalancer.UpdateStats(record.IntermediateProvider, false, 0)
            r.loadBalancer.UpdateStats(record.FinalProvider, false, 0)
            r.loadBalancer.IncrementActiveCalls(record.IntermediateProvider, -1)
            r.loadBalancer.IncrementActiveCalls(record.FinalProvider, -1)
            
            // Update call record
            record.Status = "ABANDONED"
            record.CurrentStep = "CLEANUP"
            endTime := time.Now()
            record.EndTime = &endTime
            r.updateCallRecord(record)
            
            // Remove from maps
            delete(r.activeCalls, callID)
            delete(r.didToCall, record.AssignedDID)
        }
    }
}

func (r *Router) GetStatistics() map[string]interface{} {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    stats := make(map[string]interface{})
    stats["active_calls"] = len(r.activeCalls)
    
    // Get DID statistics
    var totalDIDs, usedDIDs int
    db.DB.QueryRow("SELECT COUNT(*), SUM(CASE WHEN in_use = 1 THEN 1 ELSE 0 END) FROM dids").Scan(&totalDIDs, &usedDIDs)
    
    stats["total_dids"] = totalDIDs
    stats["used_dids"] = usedDIDs
    stats["available_dids"] = totalDIDs - usedDIDs
    
    // Get call statistics by provider
    providerStats := make(map[string]map[string]int)
    for _, record := range r.activeCalls {
        if _, exists := providerStats[record.InboundProvider]; !exists {
            providerStats[record.InboundProvider] = make(map[string]int)
        }
        providerStats[record.InboundProvider]["inbound"]++
        
        if _, exists := providerStats[record.IntermediateProvider]; !exists {
            providerStats[record.IntermediateProvider] = make(map[string]int)
        }
        providerStats[record.IntermediateProvider]["intermediate"]++
        
        if _, exists := providerStats[record.FinalProvider]; !exists {
            providerStats[record.FinalProvider] = make(map[string]int)
        }
        providerStats[record.FinalProvider]["final"]++
    }
    
    stats["provider_stats"] = providerStats
    
    return stats
}
