package agi

import (
    "bufio"
    "fmt"
    "log"
    "net"
    "strings"
    "sync"
    "time"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/router"
)

// AGI response codes
const (
    AGI_SUCCESS = "200 result=1"
    AGI_FAILURE = "200 result=0"
    AGI_ERROR   = "510 Invalid or unknown command"
)

// Server represents the AGI server
type Server struct {
    router       *router.Router
    listenPort   int
    listener     net.Listener
    connections  sync.WaitGroup
    shutdown     chan struct{}
    activeConns  sync.Map // Track active connections for monitoring
}

// AGISession represents a single AGI session
type AGISession struct {
    conn     net.Conn
    reader   *bufio.Reader
    writer   *bufio.Writer
    headers  map[string]string
    server   *Server
    id       string
    startTime time.Time
}

// NewServer creates a new AGI server instance
func NewServer(router *router.Router, port int) *Server {
    return &Server{
        router:     router,
        listenPort: port,
        shutdown:   make(chan struct{}),
    }
}

// Start starts the AGI server
func (s *Server) Start() error {
    var err error
    s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.listenPort))
    if err != nil {
        return fmt.Errorf("failed to listen on port %d: %v", s.listenPort, err)
    }
    
    log.Printf("[AGI] Server listening on port %d", s.listenPort)
    log.Printf("[AGI] Ready to process calls...")
    
    // Accept connections
    for {
        select {
        case <-s.shutdown:
            log.Println("[AGI] Server shutting down...")
            return nil
        default:
            // Set accept timeout to check shutdown periodically
            s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))
            conn, err := s.listener.Accept()
            if err != nil {
                if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                    continue
                }
                log.Printf("[AGI] Error accepting connection: %v", err)
                continue
            }
            
            s.connections.Add(1)
            go s.handleConnection(conn)
        }
    }
}

// Stop gracefully stops the AGI server
func (s *Server) Stop() {
    close(s.shutdown)
    if s.listener != nil {
        s.listener.Close()
    }
    s.connections.Wait()
    log.Println("[AGI] Server stopped")
}

// handleConnection handles a single AGI connection
func (s *Server) handleConnection(conn net.Conn) {
    defer s.connections.Done()
    
    session := &AGISession{
        conn:      conn,
        reader:    bufio.NewReader(conn),
        writer:    bufio.NewWriter(conn),
        headers:   make(map[string]string),
        server:    s,
        id:        fmt.Sprintf("%s-%d", conn.RemoteAddr().String(), time.Now().UnixNano()),
        startTime: time.Now(),
    }
    
    // Track active connection
    s.activeConns.Store(session.id, session)
    defer s.activeConns.Delete(session.id)
    
    defer session.close()
    
    log.Printf("[AGI] New connection from %s (ID: %s)", conn.RemoteAddr(), session.id)
    
    // Read AGI headers
    if err := session.readHeaders(); err != nil {
        log.Printf("[AGI] Error reading headers: %v", err)
        return
    }
    
    // Log session details
    session.logSessionStart()
    
    // Process the AGI request
    session.processRequest()
    
    // Log session end
    duration := time.Since(session.startTime)
    log.Printf("[AGI] Session %s completed (Duration: %v)", session.id, duration)
}

// readHeaders reads AGI headers from the connection
func (s *AGISession) readHeaders() error {
    log.Printf("[AGI] Reading headers for session %s", s.id)
    
    for {
        line, err := s.reader.ReadString('\n')
        if err != nil {
            return fmt.Errorf("error reading header: %v", err)
        }
        
        line = strings.TrimSpace(line)
        
        // Empty line indicates end of headers
        if line == "" {
            break
        }
        
        // Parse header
        parts := strings.SplitN(line, ":", 2)
        if len(parts) == 2 {
            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])
            s.headers[key] = value
            
            // Log important headers
            if strings.Contains(key, "agi_") {
                log.Printf("[AGI]   %s: %s", key, value)
            }
        }
    }
    
    return nil
}

// logSessionStart logs the start of an AGI session with details
func (s *AGISession) logSessionStart() {
    log.Printf("\n[AGI] ========== SESSION START ==========")
    log.Printf("[AGI] Session ID: %s", s.id)
    log.Printf("[AGI] Request: %s", s.headers["agi_request"])
    log.Printf("[AGI] Channel: %s", s.headers["agi_channel"])
    log.Printf("[AGI] CallerID: %s", s.headers["agi_callerid"])
    log.Printf("[AGI] Extension: %s", s.headers["agi_extension"])
    log.Printf("[AGI] Context: %s", s.headers["agi_context"])
    log.Printf("[AGI] UniqueID: %s", s.headers["agi_uniqueid"])
    log.Printf("[AGI] =====================================\n")
}

// processRequest processes the AGI request based on the request type
func (s *AGISession) processRequest() {
    request := s.headers["agi_request"]
    if request == "" {
        log.Printf("[AGI] No request found in headers")
        s.sendResponse(AGI_FAILURE)
        return
    }
    
    // Extract request type from AGI request
    log.Printf("[AGI] Processing request: %s", request)
    
    switch {
    case strings.Contains(request, "processIncoming"):
        s.handleIncomingCall()
    case strings.Contains(request, "processReturn"):
        s.handleReturnCall()
    case strings.Contains(request, "processFinal"):
        s.handleFinalCall()
    case strings.Contains(request, "hangup"):
        s.handleHangup()
    default:
        log.Printf("[AGI] Unknown request type: %s", request)
        s.sendResponse(AGI_FAILURE)
    }
}

// handleIncomingCall handles incoming calls from S1
func (s *AGISession) handleIncomingCall() {
    log.Printf("[AGI] Processing incoming call from S1")
    
    // Extract call information
    callID := s.headers["agi_uniqueid"]
    ani := s.headers["agi_callerid"]
    dnis := s.headers["agi_extension"]
    channel := s.headers["agi_channel"]
    
    // Extract provider from channel
    inboundProvider := s.extractProviderFromChannel(channel)
    
    log.Printf("[AGI] Incoming Call Details:")
    log.Printf("[AGI]   CallID: %s", callID)
    log.Printf("[AGI]   ANI-1: %s", ani)
    log.Printf("[AGI]   DNIS-1: %s", dnis)
    log.Printf("[AGI]   Inbound Provider: %s", inboundProvider)
    
    // Process through router
    response, err := s.server.router.ProcessIncomingCall(callID, ani, dnis, inboundProvider)
    
    if err != nil {
        log.Printf("[AGI] ERROR: Failed to process incoming call: %v", err)
        s.setVariable("ROUTER_STATUS", "failed")
        s.setVariable("ROUTER_ERROR", err.Error())
        s.sendResponse(AGI_SUCCESS)
        return
    }
    
    // Set channel variables for dialplan
    log.Printf("[AGI] Setting channel variables for routing:")
    log.Printf("[AGI]   ROUTER_STATUS = success")
    log.Printf("[AGI]   DID_ASSIGNED = %s", response.DIDAssigned)
    log.Printf("[AGI]   NEXT_HOP = %s", response.NextHop)
    log.Printf("[AGI]   ANI_TO_SEND = %s", response.ANIToSend)
    log.Printf("[AGI]   DNIS_TO_SEND = %s", response.DNISToSend)
    
    s.setVariable("ROUTER_STATUS", "success")
    s.setVariable("DID_ASSIGNED", response.DIDAssigned)
    s.setVariable("NEXT_HOP", response.NextHop)
    s.setVariable("ANI_TO_SEND", response.ANIToSend)
    s.setVariable("DNIS_TO_SEND", response.DNISToSend)
    
    s.sendResponse(AGI_SUCCESS)
    
    log.Printf("[AGI] Incoming call processed successfully")
}

// handleReturnCall handles calls returning from S3
func (s *AGISession) handleReturnCall() {
    log.Printf("[AGI] Processing return call from S3")
    
    // Extract call information
    ani2 := s.headers["agi_callerid"]
    did := s.headers["agi_extension"]
    channel := s.headers["agi_channel"]
    
    // Get source IP from channel variable
    sourceIP := s.getVariable("SOURCE_IP")
    
    // Extract provider from channel
    intermediateProvider := s.extractProviderFromChannel(channel)
    
    log.Printf("[AGI] Return Call Details:")
    log.Printf("[AGI]   ANI-2: %s", ani2)
    log.Printf("[AGI]   DID: %s", did)
    log.Printf("[AGI]   Intermediate Provider: %s", intermediateProvider)
    log.Printf("[AGI]   Source IP: %s", sourceIP)
    
    // Process through router
    response, err := s.server.router.ProcessReturnCall(ani2, did, intermediateProvider, sourceIP)
    
    if err != nil {
        log.Printf("[AGI] ERROR: Failed to process return call: %v", err)
        s.setVariable("ROUTER_STATUS", "failed")
        s.setVariable("ROUTER_ERROR", err.Error())
        s.sendResponse(AGI_SUCCESS)
        return
    }
    
    // Set channel variables for routing to S4
    log.Printf("[AGI] Setting channel variables for S4 routing:")
    log.Printf("[AGI]   ROUTER_STATUS = success")
    log.Printf("[AGI]   NEXT_HOP = %s", response.NextHop)
    log.Printf("[AGI]   ANI_TO_SEND = %s", response.ANIToSend)
    log.Printf("[AGI]   DNIS_TO_SEND = %s", response.DNISToSend)
    
    s.setVariable("ROUTER_STATUS", "success")
    s.setVariable("NEXT_HOP", response.NextHop)
    s.setVariable("ANI_TO_SEND", response.ANIToSend)
    s.setVariable("DNIS_TO_SEND", response.DNISToSend)
    
    s.sendResponse(AGI_SUCCESS)
    
    log.Printf("[AGI] Return call processed successfully")
}

// handleFinalCall handles the final call from S4
func (s *AGISession) handleFinalCall() {
    log.Printf("[AGI] Processing final call from S4")
    
    // Extract call information
    callID := s.headers["agi_uniqueid"]
    ani := s.headers["agi_callerid"]
    dnis := s.headers["agi_extension"]
    channel := s.headers["agi_channel"]
    
    // Get source IP from channel variable
    sourceIP := s.getVariable("SOURCE_IP")
    
    // Extract provider from channel
    finalProvider := s.extractProviderFromChannel(channel)
    
    log.Printf("[AGI] Final Call Details:")
    log.Printf("[AGI]   CallID: %s", callID)
    log.Printf("[AGI]   ANI: %s", ani)
    log.Printf("[AGI]   DNIS: %s", dnis)
    log.Printf("[AGI]   Final Provider: %s", finalProvider)
    log.Printf("[AGI]   Source IP: %s", sourceIP)
    
    // Process through router
    err := s.server.router.ProcessFinalCall(callID, ani, dnis, finalProvider, sourceIP)
    
    if err != nil {
        log.Printf("[AGI] ERROR: Failed to process final call: %v", err)
    } else {
        log.Printf("[AGI] Final call processed successfully")
    }
    
    s.sendResponse(AGI_SUCCESS)
}

// handleHangup handles call hangup
func (s *AGISession) handleHangup() {
    callID := s.headers["agi_uniqueid"]
    log.Printf("[AGI] Processing hangup for call %s", callID)
    
    // Additional cleanup can be added here if needed
    
    s.sendResponse(AGI_SUCCESS)
}

// setVariable sets a channel variable
func (s *AGISession) setVariable(name, value string) error {
    cmd := fmt.Sprintf("SET VARIABLE %s \"%s\"", name, value)
    log.Printf("[AGI] Executing: %s", cmd)
    
    if err := s.sendCommand(cmd); err != nil {
        return err
    }
    
    response, err := s.readResponse()
    if err != nil {
        return err
    }
    
    log.Printf("[AGI] Response: %s", response)
    return nil
}

// getVariable gets a channel variable
func (s *AGISession) getVariable(name string) string {
    cmd := fmt.Sprintf("GET VARIABLE %s", name)
    log.Printf("[AGI] Executing: %s", cmd)
    
    if err := s.sendCommand(cmd); err != nil {
        return ""
    }
    
    response, err := s.readResponse()
    if err != nil {
        return ""
    }
    
    log.Printf("[AGI] Response: %s", response)
    
    // Parse response: "200 result=1 (value)"
    if strings.Contains(response, "result=1") {
        start := strings.Index(response, "(")
        end := strings.LastIndex(response, ")")
        if start > 0 && end > start {
            value := response[start+1 : end]
            log.Printf("[AGI] Variable %s = %s", name, value)
            return value
        }
    }
    
    return ""
}

// sendCommand sends a command to Asterisk
func (s *AGISession) sendCommand(cmd string) error {
    _, err := s.writer.WriteString(cmd + "\n")
    if err != nil {
        return err
    }
    return s.writer.Flush()
}

// readResponse reads a response from Asterisk
func (s *AGISession) readResponse() (string, error) {
    response, err := s.reader.ReadString('\n')
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(response), nil
}

// sendResponse sends a simple response
func (s *AGISession) sendResponse(response string) {
    s.writer.WriteString(response + "\n")
    s.writer.Flush()
}

// extractProviderFromChannel extracts provider name from channel string
func (s *AGISession) extractProviderFromChannel(channel string) string {
    // Channel format: "PJSIP/endpoint-provider1-00000001"
    // Extract: "provider1"
    
    if channel == "" {
        return ""
    }
    
    // Remove technology prefix
    parts := strings.Split(channel, "/")
    if len(parts) < 2 {
        return ""
    }
    
    // Get endpoint part
    endpointPart := parts[1]
    
    // Extract provider name
    // Format: "endpoint-providername-uniqueid"
    endpointParts := strings.Split(endpointPart, "-")
    if len(endpointParts) >= 2 && endpointParts[0] == "endpoint" {
        // Return everything between "endpoint-" and the last "-"
        if len(endpointParts) >= 3 {
            // Join all parts except first and last
            providerParts := endpointParts[1 : len(endpointParts)-1]
            return strings.Join(providerParts, "-")
        }
        return endpointParts[1]
    }
    
    return ""
}

// close closes the AGI session
func (s *AGISession) close() {
    if s.conn != nil {
        s.conn.Close()
    }
}

// GetStats returns server statistics
func (s *Server) GetStats() map[string]interface{} {
    stats := make(map[string]interface{})
    
    activeCount := 0
    s.activeConns.Range(func(key, value interface{}) bool {
        activeCount++
        return true
    })
    
    stats["active_connections"] = activeCount
    stats["port"] = s.listenPort
    
    return stats
}
