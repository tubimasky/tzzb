package hexinv

// shimsJS 是跑 chameleon 所需的最小浏览器环境。
//
// chameleon 只在初始化时读这些对象（UA、平台、插件、canvas 等做指纹用），
// 并不真的发请求 —— 真正的网络请求仍由 Go 的 net/http 发出，所以这里
// 只需要"看起来像浏览器"，不需要任何真实渲染能力。
const shimsJS = `
var __tokens = [];
var __cookies = {};

var document = {
	getElementsByTagName: function () { return []; },
	createElement: function (t) {
		return {
			style: {}, getContext: function () { return null; },
			tagName: String(t).toUpperCase(),
			appendChild: function () {}, setAttribute: function () {},
			getElementsByTagName: function () { return []; }
		};
	},
	addEventListener: function () {}, removeEventListener: function () {},
	attachEvent: function () {}, detachEvent: function () {},
	createEvent: function () { return { initEvent: function () {} }; },
	documentElement: {}, body: {}, head: {},
	referrer: '', readyState: 'complete',
	getElementById: function () { return null; },
	querySelectorAll: function () { return []; }
};

Object.defineProperty(document, 'cookie', {
	get: function () {
		var out = [];
		for (var k in __cookies) { out.push(k + '=' + __cookies[k]); }
		return out.join('; ');
	},
	set: function (v) {
		var s = String(v);
		s.split(';').forEach(function (seg) {
			var i = seg.indexOf('=');
			if (i > 0) { __cookies[seg.slice(0, i).trim()] = seg.slice(i + 1).trim(); }
		});
		// chameleon 每生成一个新 token 就写一次 cookie v=
		var m = /(^|;\s*)v=([^;]+)/.exec(s);
		if (m) { __tokens.push(m[2]); }
	}
});

var navigator = {
	userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36',
	platform: 'Win32', language: 'zh-CN', languages: ['zh-CN'], vendor: 'Google Inc.',
	plugins: [], appName: 'Netscape', appVersion: '5.0',
	product: 'Gecko', productSub: '20030107', appCodeName: 'Mozilla',
	cookieEnabled: true, onLine: true, doNotTrack: null,
	javaEnabled: function () { return false; },
	getGamepads: function () { return []; },
	sendBeacon: function () { return true; },
	maxTouchPoints: 0, hardwareConcurrency: 8, deviceMemory: 8,
	mimeTypes: [],
	connection: { effectiveType: '4g', rtt: 50, downlink: 10 }
};

var location = {
	href: 'https://tzzb.10jqka.com.cn/pc/index.html',
	host: 'tzzb.10jqka.com.cn', hostname: 'tzzb.10jqka.com.cn',
	protocol: 'https:', pathname: '/pc/index.html', search: '', hash: '',
	origin: 'https://tzzb.10jqka.com.cn'
};

// chameleon 会 hook 这个构造器的 open/send，在 open 时生成 token。
function XMLHttpRequest() { this.readyState = 0; this.status = 200; this._h = {}; }
XMLHttpRequest.prototype.open = function () { this.readyState = 1; };
XMLHttpRequest.prototype.send = function () { this.readyState = 4; };
XMLHttpRequest.prototype.setRequestHeader = function (k, v) { this._h[k] = v; };
XMLHttpRequest.prototype.getAllResponseHeaders = function () { return ''; };
XMLHttpRequest.prototype.getResponseHeader = function () { return null; };
XMLHttpRequest.prototype.addEventListener = function () {};
XMLHttpRequest.prototype.removeEventListener = function () {};
XMLHttpRequest.prototype.abort = function () {};

var window = this;
var self = this;
var screen = { width: 1920, height: 1080, availWidth: 1920, availHeight: 1040, colorDepth: 24 };
var chrome = { runtime: {} };
var performance = { now: function () { return 1; } };
var localStorage = { getItem: function () { return null; }, setItem: function () {}, removeItem: function () {} };
var sessionStorage = localStorage;
var history = { pushState: function () {}, replaceState: function () {}, length: 1 };
var matchMedia = function () { return { matches: false, addListener: function () {}, removeListener: function () {} }; };
var getComputedStyle = function () { return { getPropertyValue: function () { return ''; } }; };
var addEventListener = function () {};
var removeEventListener = function () {};
var alert = function () {};
var confirm = function () { return true; };
var open = function () { return null; };

// DOM 类型：chameleon 用 instanceof 做类型判断，缺了会在初始化时抛错
function EventTarget() {}
function Node() {}
function Element() {}
function HTMLElement() {}
function HTMLDocument() {}
function HTMLBodyElement() {}
function HTMLHeadElement() {}
function HTMLDivElement() {}
function HTMLScriptElement() {}
function HTMLIFrameElement() {}
function HTMLCanvasElement() {}
function HTMLUnknownElement() {}
function Window() {}
function Navigator() {}
function Screen() {}
function History() {}
function Location() {}
function Storage() {}
function Document() {}
function HTMLCollection() {}
function NodeList() {}
function Event() {}
function CustomEvent() {}
function MouseEvent() {}
function KeyboardEvent() {}
function MutationObserver() {}
function Image() {}
function Audio() {}
function Option() {}
function FormData() {}
function Blob() {}
function URLSearchParams() {}
function URL() {}
function WebSocket() {}
function Worker() {}
function MessageChannel() {}
function IntersectionObserver() {}
function ResizeObserver() {}
function XMLHttpRequestEventTarget() {}

var crypto = {
	getRandomValues: function (arr) {
		for (var i = 0; i < arr.length; i++) { arr[i] = Math.floor(Math.random() * 256); }
		return arr;
	}
};
`
