package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"
)

// StealthScript contains JavaScript to evade common bot detection techniques.
// This is based on puppeteer-extra-plugin-stealth evasions and the refyne CLI's stealth.go.
const StealthScript = `
(function() {
    'use strict';

    // 1. Remove navigator.webdriver
    // Chrome 89+ already sets this to false/undefined in some cases, but we ensure it
    Object.defineProperty(navigator, 'webdriver', {
        get: () => undefined,
        configurable: true
    });
    // Also delete from prototype for older detection methods
    try {
        delete Object.getPrototypeOf(navigator).webdriver;
    } catch (e) {}

    // 2. Mock navigator.plugins with realistic values
    // Headless Chrome has an empty plugins array which is a dead giveaway
    const mockPlugins = [
        {
            name: 'Chrome PDF Plugin',
            description: 'Portable Document Format',
            filename: 'internal-pdf-viewer',
            length: 1
        },
        {
            name: 'Chrome PDF Viewer',
            description: '',
            filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai',
            length: 1
        },
        {
            name: 'Native Client',
            description: '',
            filename: 'internal-nacl-plugin',
            length: 2
        }
    ];

    try {
        const pluginArray = Object.create(PluginArray.prototype);
        mockPlugins.forEach((p, i) => {
            const plugin = Object.create(Plugin.prototype);
            Object.defineProperties(plugin, {
                name: { value: p.name, enumerable: true },
                description: { value: p.description, enumerable: true },
                filename: { value: p.filename, enumerable: true },
                length: { value: p.length, enumerable: true }
            });
            pluginArray[i] = plugin;
            pluginArray[p.name] = plugin;
        });
        Object.defineProperty(pluginArray, 'length', { value: mockPlugins.length });
        Object.defineProperty(pluginArray, 'item', { value: (i) => pluginArray[i] || null });
        Object.defineProperty(pluginArray, 'namedItem', { value: (n) => pluginArray[n] || null });
        Object.defineProperty(pluginArray, 'refresh', { value: () => {} });

        Object.defineProperty(navigator, 'plugins', {
            get: () => pluginArray,
            configurable: true
        });
    } catch (e) {}

    // 3. Mock navigator.mimeTypes
    try {
        const mockMimeTypes = [
            { type: 'application/pdf', description: 'Portable Document Format', suffixes: 'pdf' },
            { type: 'text/pdf', description: 'Portable Document Format', suffixes: 'pdf' }
        ];

        const mimeTypeArray = Object.create(MimeTypeArray.prototype);
        mockMimeTypes.forEach((m, i) => {
            const mimeType = Object.create(MimeType.prototype);
            Object.defineProperties(mimeType, {
                type: { value: m.type, enumerable: true },
                description: { value: m.description, enumerable: true },
                suffixes: { value: m.suffixes, enumerable: true },
                enabledPlugin: { value: navigator.plugins[0], enumerable: true }
            });
            mimeTypeArray[i] = mimeType;
            mimeTypeArray[m.type] = mimeType;
        });
        Object.defineProperty(mimeTypeArray, 'length', { value: mockMimeTypes.length });
        Object.defineProperty(mimeTypeArray, 'item', { value: (i) => mimeTypeArray[i] || null });
        Object.defineProperty(mimeTypeArray, 'namedItem', { value: (n) => mimeTypeArray[n] || null });

        Object.defineProperty(navigator, 'mimeTypes', {
            get: () => mimeTypeArray,
            configurable: true
        });
    } catch (e) {}

    // 4. Set navigator.languages
    Object.defineProperty(navigator, 'languages', {
        get: () => Object.freeze(['en-US', 'en']),
        configurable: true
    });

    // 5. Mock chrome.runtime
    // Headless Chrome doesn't have window.chrome in some contexts
    if (!window.chrome) {
        Object.defineProperty(window, 'chrome', {
            value: {},
            writable: true,
            enumerable: true,
            configurable: false
        });
    }

    if (!window.chrome.runtime) {
        window.chrome.runtime = {
            OnInstalledReason: {
                CHROME_UPDATE: 'chrome_update',
                INSTALL: 'install',
                SHARED_MODULE_UPDATE: 'shared_module_update',
                UPDATE: 'update'
            },
            OnRestartRequiredReason: {
                APP_UPDATE: 'app_update',
                OS_UPDATE: 'os_update',
                PERIODIC: 'periodic'
            },
            PlatformArch: {
                ARM: 'arm',
                ARM64: 'arm64',
                MIPS: 'mips',
                MIPS64: 'mips64',
                X86_32: 'x86-32',
                X86_64: 'x86-64'
            },
            PlatformNaclArch: {
                ARM: 'arm',
                MIPS: 'mips',
                MIPS64: 'mips64',
                X86_32: 'x86-32',
                X86_64: 'x86-64'
            },
            PlatformOs: {
                ANDROID: 'android',
                CROS: 'cros',
                LINUX: 'linux',
                MAC: 'mac',
                OPENBSD: 'openbsd',
                WIN: 'win'
            },
            RequestUpdateCheckStatus: {
                NO_UPDATE: 'no_update',
                THROTTLED: 'throttled',
                UPDATE_AVAILABLE: 'update_available'
            },
            get id() { return undefined; },
            connect: function() {},
            sendMessage: function() {}
        };
    }

    // 6. Mock chrome.csi (Chrome client-side instrumentation)
    if (!window.chrome.csi) {
        window.chrome.csi = function() {
            return {
                onloadT: Date.now(),
                startE: Date.now(),
                pageT: Math.random() * 1000,
                tran: 15
            };
        };
    }

    // 7. Mock chrome.loadTimes
    if (!window.chrome.loadTimes) {
        window.chrome.loadTimes = function() {
            return {
                requestTime: Date.now() / 1000,
                startLoadTime: Date.now() / 1000,
                commitLoadTime: Date.now() / 1000 + Math.random(),
                finishDocumentLoadTime: Date.now() / 1000 + Math.random(),
                finishLoadTime: Date.now() / 1000 + Math.random(),
                firstPaintTime: Date.now() / 1000 + Math.random(),
                firstPaintAfterLoadTime: 0,
                navigationType: 'Navigate',
                wasFetchedViaSpdy: false,
                wasNpnNegotiated: true,
                npnNegotiatedProtocol: 'h2',
                wasAlternateProtocolAvailable: false,
                connectionInfo: 'h2'
            };
        };
    }

    // 8. Fix permissions query for notifications
    try {
        const originalQuery = Permissions.prototype.query;
        Permissions.prototype.query = function(parameters) {
            if (parameters.name === 'notifications') {
                return Promise.resolve({ state: Notification.permission });
            }
            return originalQuery.call(this, parameters);
        };
    } catch (e) {}

    // 9. Override WebGL vendor and renderer
    const getParameterProxyHandler = {
        apply: function(target, ctx, args) {
            const param = args[0];
            const result = Reflect.apply(target, ctx, args);
            // UNMASKED_VENDOR_WEBGL
            if (param === 37445) {
                return 'Intel Inc.';
            }
            // UNMASKED_RENDERER_WEBGL
            if (param === 37446) {
                return 'Intel Iris OpenGL Engine';
            }
            return result;
        }
    };

    // Patch WebGL contexts
    try {
        const webglGetParameter = WebGLRenderingContext.prototype.getParameter;
        WebGLRenderingContext.prototype.getParameter = new Proxy(webglGetParameter, getParameterProxyHandler);
    } catch (e) {}

    try {
        const webgl2GetParameter = WebGL2RenderingContext.prototype.getParameter;
        WebGL2RenderingContext.prototype.getParameter = new Proxy(webgl2GetParameter, getParameterProxyHandler);
    } catch (e) {}

    // 10. Fix iframe contentWindow access
    try {
        Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
            get: function() {
                return this.contentDocument?.defaultView || null;
            }
        });
    } catch (e) {}

    // 11. Make toString() for native functions look native
    try {
        const nativeToStringFunc = Function.prototype.toString;
        const customToString = function() {
            if (this === Permissions.prototype.query) {
                return 'function query() { [native code] }';
            }
            return nativeToStringFunc.call(this);
        };
        Function.prototype.toString = customToString;
    } catch (e) {}

    // 12. Override navigator.hardwareConcurrency if it's 0 (suspicious in headless)
    if (navigator.hardwareConcurrency === 0 || navigator.hardwareConcurrency === undefined) {
        Object.defineProperty(navigator, 'hardwareConcurrency', {
            get: () => 4,
            configurable: true
        });
    }

    // 13. Override navigator.deviceMemory if missing
    if (navigator.deviceMemory === undefined || navigator.deviceMemory === 0) {
        Object.defineProperty(navigator, 'deviceMemory', {
            get: () => 8,
            configurable: true
        });
    }

    // 14. Mock navigator.connection
    if (!navigator.connection) {
        Object.defineProperty(navigator, 'connection', {
            get: () => ({
                effectiveType: '4g',
                rtt: 100,
                downlink: 10,
                saveData: false
            }),
            configurable: true
        });
    }

    // 15. Mock navigator.getBattery
    if (!navigator.getBattery) {
        navigator.getBattery = function() {
            return Promise.resolve({
                charging: true,
                chargingTime: 0,
                dischargingTime: Infinity,
                level: 1.0,
                addEventListener: function() {},
                removeEventListener: function() {}
            });
        };
    }
})();
`

// CreateStealthPage creates a new page with stealth patches applied.
// This uses go-rod/stealth which embeds puppeteer-extra-plugin-stealth evasions.
func CreateStealthPage(browser *rod.Browser) (*rod.Page, error) {
	// Use go-rod/stealth to create a stealth page
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, err
	}

	// Additionally inject our custom stealth script for extra coverage
	if _, err := page.EvalOnNewDocument(StealthScript); err != nil {
		page.Close()
		return nil, err
	}

	return page, nil
}

// MustCreateStealthPage creates a stealth page, panicking on error.
func MustCreateStealthPage(browser *rod.Browser) *rod.Page {
	page, err := CreateStealthPage(browser)
	if err != nil {
		panic(err)
	}
	return page
}
