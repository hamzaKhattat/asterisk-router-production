package ami

import (
    "bufio"
    "fmt"
    "log"
    "net"
    "strings"
    "sync"
    "time"
)

type Manager struct {
    host     string
    port     int
    username string
    password string
    conn     net.Conn
    reader   *bufio.Reader
    writer   *bufio.Writer
    mu       sync.Mutex
    eventCh  chan Event
}

type Event map[string]string

func NewManager(host string, port int, username, password string) *Manager {
	fmt.Println(time.Now())
    return &Manager{
        host:     host,
        port:     port,
        username: username,
        password: password,
        eventCh:  make(chan Event, 100),
    }
}

func (m *Manager) Connect() error {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", m.host, m.port))
    if err != nil {
        return err
    }
    
    m.conn = conn
    m.reader = bufio.NewReader(conn)
    m.writer = bufio.NewWriter(conn)
    
    // Read welcome message
    if _, err := m.reader.ReadString('\n'); err != nil {
        return err
    }
    
    // Login
    if err := m.login(); err != nil {
        return err
    }
    
    // Start event reader
    go m.eventReader()
    
    log.Println("AMI connected successfully")
    return nil
}

func (m *Manager) login() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    loginCmd := fmt.Sprintf("Action: Login\r\nUsername: %s\r\nSecret: %s\r\n\r\n", 
        m.username, m.password)
    
    if _, err := m.writer.WriteString(loginCmd); err != nil {
        return err
    }
    
    if err := m.writer.Flush(); err != nil {
        return err
    }
    
    // Read response
    response := m.readResponse()
    if response["Response"] != "Success" {
        return fmt.Errorf("login failed: %s", response["Message"])
    }
    
    return nil
}

func (m *Manager) readResponse() Event {
    event := make(Event)
    
    for {
        line, err := m.reader.ReadString('\n')
        if err != nil {
            break
        }
        
        line = strings.TrimSpace(line)
        if line == "" {
            break
        }
        
        parts := strings.SplitN(line, ":", 2)
        if len(parts) == 2 {
            event[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
        }
    }
    
    return event
}

func (m *Manager) eventReader() {
    for {
        event := m.readResponse()
        if len(event) > 0 {
            select {
            case m.eventCh <- event:
            default:
                // Channel full, drop event
            }
        }
    }
}

func (m *Manager) Originate(channel, context, exten, priority, callerID string, timeout int, variables map[string]string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cmd := fmt.Sprintf("Action: Originate\r\nChannel: %s\r\nContext: %s\r\nExten: %s\r\nPriority: %s\r\nCallerID: %s\r\nTimeout: %d\r\n",
        channel, context, exten, priority, callerID, timeout)
    
    for k, v := range variables {
        cmd += fmt.Sprintf("Variable: %s=%s\r\n", k, v)
    }
    
    cmd += "\r\n"
    
    if _, err := m.writer.WriteString(cmd); err != nil {
        return err
    }
    
    return m.writer.Flush()
}

func (m *Manager) Command(command string) (string, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cmd := fmt.Sprintf("Action: Command\r\nCommand: %s\r\n\r\n", command)
    
    if _, err := m.writer.WriteString(cmd); err != nil {
        return "", err
    }
    
    if err := m.writer.Flush(); err != nil {
        return "", err
    }
    
    response := m.readResponse()
    return response["Output"], nil
}

func (m *Manager) ReloadModule(module string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cmd := fmt.Sprintf("Action: Reload\r\nModule: %s\r\n\r\n", module)
    
    if _, err := m.writer.WriteString(cmd); err != nil {
        return err
    }
    
    if err := m.writer.Flush(); err != nil {
        return err
    }
    
    response := m.readResponse()
    if response["Response"] != "Success" {
        return fmt.Errorf("reload failed: %s", response["Message"])
    }
    
    return nil
}

func (m *Manager) Close() {
    if m.conn != nil {
        m.conn.Close()
    }
}

func (m *Manager) Events() <-chan Event {
    return m.eventCh
}

// Utility functions for channel operations
func (m *Manager) GetChannelStatus(channel string) (map[string]string, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cmd := fmt.Sprintf("Action: Status\r\nChannel: %s\r\n\r\n", channel)
    
    if _, err := m.writer.WriteString(cmd); err != nil {
        return nil, err
    }
    
    if err := m.writer.Flush(); err != nil {
        return nil, err
    }
    
    response := m.readResponse()
    return response, nil
}

func (m *Manager) HangupChannel(channel string, cause int) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cmd := fmt.Sprintf("Action: Hangup\r\nChannel: %s\r\nCause: %d\r\n\r\n", channel, cause)
    
    if _, err := m.writer.WriteString(cmd); err != nil {
        return err
    }
    
    return m.writer.Flush()
}
