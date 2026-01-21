package main

import (
	"context"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appnet-org/arpc/cmd/proxy-buffer/util"
	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
)

const (
	// ElementPluginDir is the fixed directory where element plugins are stored
	ElementPluginDir = "/appnet/arpc-plugins"
	// ElementPluginPrefix is the prefix for element plugin files
	ElementPluginPrefix = "element-"
)

var (
	// currentElementChain is stored in an atomic.Value for lock-free reads
	currentElementChain  atomic.Value // *RPCElementChain
	highestElementFile   string
	highestElementFileMu sync.Mutex // Protects highestElementFile
	pluginInterface      elementInit
	pluginInterfaceMu    sync.Mutex // Protects pluginInterface
	elementPluginPrefix  string
)

// elementInit is the interface that element plugins must implement
type elementInit interface {
	Element() RPCElement
	Kill() // Optional: for cleanup if plugin has background goroutines
	Init()
}

// pluginElementInitWrapper wraps a plugin's ElementInit to adapt it to our elementInit interface
// This is needed because plugins define their own RPCElement type which is different
// from main.RPCElement, even though they have the same methods
type pluginElementInitWrapper struct {
	pluginInit interface {
		Element() interface{}
		Kill()
		Init()
	}
}

func (w *pluginElementInitWrapper) Element() RPCElement {
	// Get the element from the plugin (returns plugin's RPCElement type)
	pluginElement := w.pluginInit.Element()

	// Type assert to our RPCElement interface
	// This works because both types have the same method signatures
	element, ok := pluginElement.(RPCElement)
	if !ok {
		// If direct assertion fails, try to create an adapter
		// This handles the case where the plugin's type doesn't directly match
		return &elementAdapter{elem: pluginElement}
	}
	return element
}

func (w *pluginElementInitWrapper) Kill() {
	w.pluginInit.Kill()
}

func (w *pluginElementInitWrapper) Init() {
	w.pluginInit.Init()
}

// elementAdapter adapts a plugin's element to our RPCElement interface
type elementAdapter struct {
	elem interface{}
}

func (a *elementAdapter) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	// Use type assertion to call the method
	if elem, ok := a.elem.(interface {
		ProcessRequest(context.Context, *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
	}); ok {
		return elem.ProcessRequest(ctx, packet)
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

func (a *elementAdapter) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if elem, ok := a.elem.(interface {
		ProcessResponse(context.Context, *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
	}); ok {
		return elem.ProcessResponse(ctx, packet)
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

func (a *elementAdapter) Name() string {
	if elem, ok := a.elem.(interface {
		Name() string
	}); ok {
		return elem.Name()
	}
	return "UnknownElement"
}

func init() {
	// Start background goroutine to periodically check for plugin updates
	go func() {
		for {
			if elementPluginPrefix != "" {
				updateElements(elementPluginPrefix)
			}
			time.Sleep(1000 * time.Millisecond)
		}
	}()
}

// InitElementLoader initializes the element loader with the given plugin prefix path
func InitElementLoader(pluginPrefixPath string) {
	logging.Info("Initializing element loader", zap.String("pluginPrefix", pluginPrefixPath))
	elementPluginPrefix = pluginPrefixPath
	// Do an initial load
	updateElements(pluginPrefixPath)
}

// GetElementChain returns the current element chain in a thread-safe, lock-free manner
func GetElementChain() *RPCElementChain {
	chain := currentElementChain.Load()
	if chain == nil {
		return nil
	}
	return chain.(*RPCElementChain)
}

// updateElements scans the plugin directory for element plugin files and loads the highest one
func updateElements(prefix string) {
	highestElementFileMu.Lock()
	currentHighest := highestElementFile
	highestElementFileMu.Unlock()

	var highestSeenElement string = currentHighest

	dir, prefixName := filepath.Split(prefix)
	if dir == "" {
		dir = ElementPluginDir
	}
	if prefixName == "" {
		prefixName = ElementPluginPrefix
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist or can't be read - this is okay, just log and continue
		if !os.IsNotExist(err) {
			logging.Debug("Error reading element plugin directory", zap.String("dir", dir), zap.Error(err))
		}
		// If this is the first check and no directory exists, initialize with empty chain
		if currentElementChain.Load() == nil {
			currentElementChain.Store(NewRPCElementChain())
			logging.Debug("Initialized with empty element chain (no plugin directory)")
		}
		return
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), prefixName) {
			if file.Name() > highestSeenElement {
				highestSeenElement = file.Name()
			}
		}
	}

	if highestSeenElement != currentHighest {
		highestElementFileMu.Lock()
		highestElementFile = highestSeenElement
		highestElementFileMu.Unlock()

		// If no plugin file found, create an empty chain
		if highestSeenElement == "" {
			logging.Debug("No element plugin found, using empty chain")
			currentElementChain.Store(NewRPCElementChain())
			// Kill previous plugin if it exists
			pluginInterfaceMu.Lock()
			if pluginInterface != nil {
				pluginInterface.Kill()
				pluginInterface = nil
			}
			pluginInterfaceMu.Unlock()
			return
		}

		pluginPath := filepath.Join(dir, highestSeenElement)
		elementInit := loadElementPlugin(pluginPath)
		if elementInit != nil {
			// Kill previous plugin if it exists
			pluginInterfaceMu.Lock()
			if pluginInterface != nil {
				pluginInterface.Kill()
			}
			pluginInterface = elementInit
			pluginInterfaceMu.Unlock()

			// Create new chain with the element from plugin
			element := elementInit.Element()
			elementInit.Init()
			if element != nil {
				// Store atomically - this is a lock-free write
				currentElementChain.Store(NewRPCElementChain(element))
				logging.Info("Updated element chain from plugin",
					zap.String("plugin", pluginPath),
					zap.String("element", element.Name()))
			} else {
				logging.Warn("Plugin returned nil element, keeping previous chain", zap.String("plugin", pluginPath))
			}
		} else {
			// Plugin loading failed, keep previous chain (or initialize empty if first load)
			if currentElementChain.Load() == nil {
				currentElementChain.Store(NewRPCElementChain())
				logging.Debug("Initialized with empty element chain (plugin load failed)")
			}
		}
	}
}

// loadElementPlugin loads an element plugin from the specified path
func loadElementPlugin(elementPluginPath string) elementInit {
	logging.Info("Loading element plugin", zap.String("path", elementPluginPath))

	elementPlugin, err := plugin.Open(elementPluginPath)
	if err != nil {
		logging.Error("Error loading element plugin", zap.String("path", elementPluginPath), zap.Error(err))
		return nil
	}

	symElementInit, err := elementPlugin.Lookup("ElementInit")
	if err != nil {
		logging.Error("Error locating ElementInit symbol in plugin", zap.String("path", elementPluginPath), zap.Error(err))
		return nil
	}

	// Use interface{} and type assertion with a wrapper
	// This is necessary because plugins define their own RPCElement type
	// which is different from main.RPCElement even if they have the same methods
	//
	// NOTE: plugin.Lookup returns a pointer to the exported variable.
	// If the plugin exports `var ElementInit SomeInterface = ...`, we get *SomeInterface.
	// We need to dereference it to get the actual interface value.
	actualInit := symElementInit
	if ptr, ok := symElementInit.(*interface{}); ok {
		actualInit = *ptr
	}
	pluginInit, ok := actualInit.(interface {
		Element() interface{} // Accept any type that implements the methods
		Kill()
		Init()
	})
	if !ok {
		logging.Error("Error casting ElementInit from plugin - plugin must export ElementInit with Element() and Kill() methods", zap.String("path", elementPluginPath))
		return nil
	}

	// Create a wrapper that adapts the plugin's elementInit to our elementInit interface
	wrapper := &pluginElementInitWrapper{
		pluginInit: pluginInit,
	}

	logging.Info("Successfully loaded element plugin", zap.String("path", elementPluginPath))
	return wrapper
}
