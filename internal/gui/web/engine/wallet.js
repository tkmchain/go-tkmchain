//#region \0rolldown/runtime.js
var e = Object.defineProperty, t = (t, n) => {
	let r = {};
	for (var i in t) e(r, i, {
		get: t[i],
		enumerable: !0
	});
	return n || e(r, Symbol.toStringTag, { value: "Module" }), r;
}, n = "6.17.0";
//#endregion
//#region node_modules/ethers/lib.esm/utils/properties.js
function r(e, t, n) {
	let r = t.split("|").map((e) => e.trim());
	for (let n = 0; n < r.length; n++) switch (t) {
		case "any": return;
		case "bigint":
		case "boolean":
		case "number":
		case "string": if (typeof e === t) return;
	}
	let i = /* @__PURE__ */ Error(`invalid value for type ${t}`);
	throw i.code = "INVALID_ARGUMENT", i.argument = `value.${n}`, i.value = e, i;
}
async function i(e) {
	let t = Object.keys(e);
	return (await Promise.all(t.map((t) => Promise.resolve(e[t])))).reduce((e, n, r) => (e[t[r]] = n, e), {});
}
function a(e, t, n) {
	for (let i in t) {
		let a = t[i], o = n ? n[i] : null;
		o && r(a, o, i), Object.defineProperty(e, i, {
			enumerable: !0,
			value: a,
			writable: !1
		});
	}
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/errors.js
function o(e, t) {
	if (e == null) return "null";
	if (t ??= /* @__PURE__ */ new Set(), typeof e == "object") {
		if (t.has(e)) return "[Circular]";
		t.add(e);
	}
	if (Array.isArray(e)) return "[ " + e.map((e) => o(e, t)).join(", ") + " ]";
	if (e instanceof Uint8Array) {
		let t = "0123456789abcdef", n = "0x";
		for (let r = 0; r < e.length; r++) n += t[e[r] >> 4], n += t[e[r] & 15];
		return n;
	}
	if (typeof e == "object" && typeof e.toJSON == "function") return o(e.toJSON(), t);
	switch (typeof e) {
		case "boolean":
		case "number":
		case "symbol": return e.toString();
		case "bigint": return BigInt(e).toString();
		case "string": return JSON.stringify(e);
		case "object": {
			let n = Object.keys(e);
			return n.sort(), "{ " + n.map((n) => `${o(n, t)}: ${o(e[n], t)}`).join(", ") + " }";
		}
	}
	return "[ COULD NOT SERIALIZE ]";
}
function s(e, t) {
	return e && e.code === t;
}
function c(e) {
	return s(e, "CALL_EXCEPTION");
}
function l(e, t, r) {
	let i = e;
	{
		let i = [];
		if (r) {
			if ("message" in r || "code" in r || "name" in r) throw Error(`value will overwrite populated values: ${o(r)}`);
			for (let e in r) {
				if (e === "shortMessage") continue;
				let t = r[e];
				i.push(e + "=" + o(t));
			}
		}
		i.push(`code=${t}`), i.push(`version=${n}`), i.length && (e += " (" + i.join(", ") + ")");
	}
	let s;
	switch (t) {
		case "INVALID_ARGUMENT":
			s = TypeError(e);
			break;
		case "NUMERIC_FAULT":
		case "BUFFER_OVERRUN":
			s = RangeError(e);
			break;
		default: s = Error(e);
	}
	return a(s, { code: t }), r && Object.assign(s, r), s.shortMessage ?? a(s, { shortMessage: i }), s;
}
function u(e, t, n, r) {
	if (!e) throw l(t, n, r);
}
function d(e, t, n, r) {
	u(e, t, "INVALID_ARGUMENT", {
		argument: n,
		value: r
	});
}
function f(e, t, n) {
	n ??= "", n &&= ": " + n, u(e >= t, "missing argument" + n, "MISSING_ARGUMENT", {
		count: e,
		expectedCount: t
	}), u(e <= t, "too many arguments" + n, "UNEXPECTED_ARGUMENT", {
		count: e,
		expectedCount: t
	});
}
var p = [
	"NFD",
	"NFC",
	"NFKD",
	"NFKC"
].reduce((e, t) => {
	try {
		/* c8 ignore start */
		if ("test".normalize(t) !== "test") throw Error("bad");
		/* c8 ignore stop */
		if (t === "NFD" && "é".normalize("NFD") !== "é") throw Error("broken");
		e.push(t);
	} catch {}
	return e;
}, []);
function m(e) {
	u(p.indexOf(e) >= 0, "platform missing String.prototype.normalize", "UNSUPPORTED_OPERATION", {
		operation: "String.prototype.normalize",
		info: { form: e }
	});
}
function h(e, t, n) {
	if (n ??= "", e !== t) {
		let e = n, t = "new";
		n && (e += ".", t += " " + n), u(!1, `private constructor; use ${e}from* methods`, "UNSUPPORTED_OPERATION", { operation: t });
	}
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/data.js
function g(e, t, n) {
	if (e instanceof Uint8Array) return n ? new Uint8Array(e) : e;
	if (typeof e == "string" && e.length % 2 == 0 && e.match(/^0x[0-9a-f]*$/i)) {
		let t = new Uint8Array((e.length - 2) / 2), n = 2;
		for (let r = 0; r < t.length; r++) t[r] = parseInt(e.substring(n, n + 2), 16), n += 2;
		return t;
	}
	d(!1, "invalid BytesLike value", t || "value", e);
}
function _(e, t) {
	return g(e, t, !1);
}
function v(e, t) {
	return g(e, t, !0);
}
function y(e, t) {
	return !(typeof e != "string" || !e.match(/^0x[0-9A-Fa-f]*$/) || typeof t == "number" && e.length !== 2 + 2 * t || t === !0 && e.length % 2 != 0);
}
function b(e) {
	return y(e, !0) || e instanceof Uint8Array;
}
var x = "0123456789abcdef";
function S(e) {
	let t = _(e), n = "0x";
	for (let e = 0; e < t.length; e++) {
		let r = t[e];
		n += x[(r & 240) >> 4] + x[r & 15];
	}
	return n;
}
function C(e) {
	return "0x" + e.map((e) => S(e).substring(2)).join("");
}
function w(e) {
	return y(e, !0) ? (e.length - 2) / 2 : _(e).length;
}
function T(e, t, n) {
	let r = _(e);
	return n != null && n > r.length && u(!1, "cannot slice beyond data bounds", "BUFFER_OVERRUN", {
		buffer: r,
		length: r.length,
		offset: n
	}), S(r.slice(t ?? 0, n ?? r.length));
}
function E(e, t, n) {
	let r = _(e);
	u(t >= r.length, "padding exceeds data length", "BUFFER_OVERRUN", {
		buffer: new Uint8Array(r),
		length: t,
		offset: t + 1
	});
	let i = new Uint8Array(t);
	return i.fill(0), n ? i.set(r, t - r.length) : i.set(r, 0), S(i);
}
function D(e, t) {
	return E(e, t, !0);
}
function O(e, t) {
	return E(e, t, !1);
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/maths.js
var k = BigInt(0), A = BigInt(1), j = 9007199254740991;
function M(e, t) {
	let n = I(e, "value"), r = BigInt(z(t, "width"));
	if (u(n >> r === k, "overflow", "NUMERIC_FAULT", {
		operation: "fromTwos",
		fault: "overflow",
		value: e
	}), n >> r - A) {
		let e = (A << r) - A;
		return -((~n & e) + A);
	}
	return n;
}
function N(e, t) {
	let n = F(e, "value"), r = BigInt(z(t, "width")), i = A << r - A;
	if (n < k) {
		n = -n, u(n <= i, "too low", "NUMERIC_FAULT", {
			operation: "toTwos",
			fault: "overflow",
			value: e
		});
		let t = (A << r) - A;
		return (~n & t) + A;
	}
	return u(n < i, "too high", "NUMERIC_FAULT", {
		operation: "toTwos",
		fault: "overflow",
		value: e
	}), n;
}
function P(e, t) {
	return I(e, "value") & (A << BigInt(z(t, "bits"))) - A;
}
function F(e, t) {
	switch (typeof e) {
		case "bigint": return e;
		case "number": return d(Number.isInteger(e), "underflow", t || "value", e), d(e >= -9007199254740991 && e <= j, "overflow", t || "value", e), BigInt(e);
		case "string": try {
			if (e === "") throw Error("empty string");
			return e[0] === "-" && e[1] !== "-" ? -BigInt(e.substring(1)) : BigInt(e);
		} catch (n) {
			d(!1, `invalid BigNumberish string: ${n.message}`, t || "value", e);
		}
	}
	d(!1, "invalid BigNumberish value", t || "value", e);
}
function I(e, t) {
	let n = F(e, t);
	return u(n >= k, "unsigned value cannot be negative", "NUMERIC_FAULT", {
		fault: "overflow",
		operation: "getUint",
		value: e
	}), n;
}
var L = "0123456789abcdef";
function R(e) {
	if (e instanceof Uint8Array) {
		let t = "0x0";
		for (let n of e) t += L[n >> 4], t += L[n & 15];
		return BigInt(t);
	}
	return F(e);
}
function z(e, t) {
	switch (typeof e) {
		case "bigint": return d(e >= -9007199254740991 && e <= j, "overflow", t || "value", e), Number(e);
		case "number": return d(Number.isInteger(e), "underflow", t || "value", e), d(e >= -9007199254740991 && e <= j, "overflow", t || "value", e), e;
		case "string": try {
			if (e === "") throw Error("empty string");
			return z(BigInt(e), t);
		} catch (n) {
			d(!1, `invalid numeric string: ${n.message}`, t || "value", e);
		}
	}
	d(!1, "invalid numeric value", t || "value", e);
}
function ee(e) {
	return z(R(e));
}
function te(e, t) {
	let n = I(e, "value"), r = n.toString(16);
	if (t == null) r.length % 2 && (r = "0" + r);
	else {
		let i = z(t, "width");
		if (i === 0 && n === k) return "0x";
		for (u(i * 2 >= r.length, `value exceeds width (${i} bytes)`, "NUMERIC_FAULT", {
			operation: "toBeHex",
			fault: "overflow",
			value: e
		}); r.length < i * 2;) r = "0" + r;
	}
	return "0x" + r;
}
function B(e, t) {
	let n = I(e, "value");
	if (n === k) {
		let e = t == null ? 0 : z(t, "width");
		return new Uint8Array(e);
	}
	let r = n.toString(16);
	if (r.length % 2 && (r = "0" + r), t != null) {
		let n = z(t, "width");
		for (; r.length < n * 2;) r = "00" + r;
		u(n * 2 === r.length, `value exceeds width (${n} bytes)`, "NUMERIC_FAULT", {
			operation: "toBeArray",
			fault: "overflow",
			value: e
		});
	}
	let i = new Uint8Array(r.length / 2);
	for (let e = 0; e < i.length; e++) {
		let t = e * 2;
		i[e] = parseInt(r.substring(t, t + 2), 16);
	}
	return i;
}
function V(e) {
	let t = S(b(e) ? e : B(e)).substring(2);
	for (; t.startsWith("0");) t = t.substring(1);
	return t === "" && (t = "0"), "0x" + t;
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/base58.js
var ne = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", re = BigInt(58);
function ie(e) {
	let t = _(e), n = R(t), r = "";
	for (; n;) r = ne[Number(n % re)] + r, n /= re;
	for (let e = 0; e < t.length && !t[e]; e++) r = ne[0] + r;
	return r;
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/base64-browser.js
function ae(e) {
	e = atob(e);
	let t = new Uint8Array(e.length);
	for (let n = 0; n < e.length; n++) t[n] = e.charCodeAt(n);
	return _(t);
}
function oe(e) {
	let t = _(e), n = "";
	for (let e = 0; e < t.length; e++) n += String.fromCharCode(t[e]);
	return btoa(n);
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/events.js
var se = class {
	filter;
	emitter;
	#e;
	constructor(e, t, n) {
		this.#e = t, a(this, {
			emitter: e,
			filter: n
		});
	}
	async removeListener() {
		this.#e != null && await this.emitter.off(this.filter, this.#e);
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/utils/utf8.js
function ce(e, t, n, r, i) {
	d(!1, `invalid codepoint at offset ${t}; ${e}`, "bytes", n);
}
function le(e, t, n, r, i) {
	if (e === "BAD_PREFIX" || e === "UNEXPECTED_CONTINUE") {
		let e = 0;
		for (let r = t + 1; r < n.length && n[r] >> 6 == 2; r++) e++;
		return e;
	}
	return e === "OVERRUN" ? n.length - t - 1 : 0;
}
function ue(e, t, n, r, i) {
	return e === "OVERLONG" ? (d(typeof i == "number", "invalid bad code point for replacement", "badCodepoint", i), r.push(i), 0) : (r.push(65533), le(e, t, n, r, i));
}
var de = Object.freeze({
	error: ce,
	ignore: le,
	replace: ue
});
function fe(e, t) {
	t ??= de.error;
	let n = _(e, "bytes"), r = [], i = 0;
	for (; i < n.length;) {
		let e = n[i++];
		if (!(e >> 7)) {
			r.push(e);
			continue;
		}
		let a = null, o = null;
		if ((e & 224) == 192) a = 1, o = 127;
		else if ((e & 240) == 224) a = 2, o = 2047;
		else if ((e & 248) == 240) a = 3, o = 65535;
		else {
			(e & 192) == 128 ? i += t("UNEXPECTED_CONTINUE", i - 1, n, r) : i += t("BAD_PREFIX", i - 1, n, r);
			continue;
		}
		if (i - 1 + a >= n.length) {
			i += t("OVERRUN", i - 1, n, r);
			continue;
		}
		let s = e & (1 << 8 - a - 1) - 1;
		for (let e = 0; e < a; e++) {
			let e = n[i];
			if ((e & 192) != 128) {
				i += t("MISSING_CONTINUE", i, n, r), s = null;
				break;
			}
			s = s << 6 | e & 63, i++;
		}
		if (s !== null) {
			if (s > 1114111) {
				i += t("OUT_OF_RANGE", i - 1 - a, n, r, s);
				continue;
			}
			if (s >= 55296 && s <= 57343) {
				i += t("UTF16_SURROGATE", i - 1 - a, n, r, s);
				continue;
			}
			if (s <= o) {
				i += t("OVERLONG", i - 1 - a, n, r, s);
				continue;
			}
			r.push(s);
		}
	}
	return r;
}
function H(e, t) {
	d(typeof e == "string", "invalid string value", "str", e), t != null && (m(t), e = e.normalize(t));
	let n = [];
	for (let t = 0; t < e.length; t++) {
		let r = e.charCodeAt(t);
		if (r < 128) n.push(r);
		else if (r < 2048) n.push(r >> 6 | 192), n.push(r & 63 | 128);
		else if ((r & 64512) == 55296) {
			t++;
			let i = e.charCodeAt(t);
			d(t < e.length && (i & 64512) == 56320, "invalid surrogate pair", "str", e);
			let a = 65536 + ((r & 1023) << 10) + (i & 1023);
			n.push(a >> 18 | 240), n.push(a >> 12 & 63 | 128), n.push(a >> 6 & 63 | 128), n.push(a & 63 | 128);
		} else n.push(r >> 12 | 224), n.push(r >> 6 & 63 | 128), n.push(r & 63 | 128);
	}
	return new Uint8Array(n);
}
function pe(e) {
	return e.map((e) => e <= 65535 ? String.fromCharCode(e) : (e -= 65536, String.fromCharCode((e >> 10 & 1023) + 55296, (e & 1023) + 56320))).join("");
}
function me(e, t) {
	return pe(fe(e, t));
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/geturl-browser.js
function he(e) {
	async function t(t, n) {
		u(n == null || !n.cancelled, "request cancelled before sending", "CANCELLED");
		let r = t.url.split(":")[0].toLowerCase();
		u(r === "http" || r === "https", `unsupported protocol ${r}`, "UNSUPPORTED_OPERATION", {
			info: { protocol: r },
			operation: "request"
		}), u(r === "https" || !t.credentials || t.allowInsecureAuthentication, "insecure authorized connections unsupported", "UNSUPPORTED_OPERATION", { operation: "request" });
		let i = null, a = new AbortController(), o = setTimeout(() => {
			i = l("request timeout", "TIMEOUT"), a.abort();
		}, t.timeout);
		n && n.addListener(() => {
			i = l("request cancelled", "CANCELLED"), a.abort();
		});
		let s = Object.assign({}, e, {
			method: t.method,
			headers: new Headers(Array.from(t)),
			body: t.body || void 0,
			signal: a.signal
		}), c;
		try {
			c = await fetch(t.url, s);
		} catch (e) {
			throw clearTimeout(o), i || e;
		}
		clearTimeout(o);
		let d = {};
		c.headers.forEach((e, t) => {
			d[t.toLowerCase()] = e;
		});
		let f = await c.arrayBuffer(), p = f == null ? null : new Uint8Array(f);
		return {
			statusCode: c.status,
			statusMessage: c.statusText,
			headers: d,
			body: p
		};
	}
	return t;
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/fetch.js
var ge = 12, _e = 250, ve = he(), ye = /* @__PURE__ */ RegExp("^data:([^;:]*)?(;base64)?,(.*)$", "i"), be = /* @__PURE__ */ RegExp("^ipfs://(ipfs/)?(.*)$", "i"), xe = !1;
async function Se(e, t) {
	try {
		let t = e.match(ye);
		if (!t) throw Error("invalid data");
		return new ke(200, "OK", { "content-type": t[1] || "text/plain" }, t[2] ? ae(t[3]) : je(t[3]));
	} catch {
		return new ke(599, "BAD REQUEST (invalid data: URI)", {}, null, new Oe(e));
	}
}
function Ce(e) {
	async function t(t, n) {
		try {
			let n = t.match(be);
			if (!n) throw Error("invalid link");
			return new Oe(`${e}${n[2]}`);
		} catch {
			return new ke(599, "BAD REQUEST (invalid IPFS URI)", {}, null, new Oe(t));
		}
	}
	return t;
}
var we = {
	data: Se,
	ipfs: Ce("https://gateway.ipfs.io/ipfs/")
}, Te = /* @__PURE__ */ new WeakMap(), Ee = class {
	#e;
	#t;
	constructor(e) {
		this.#e = [], this.#t = !1, Te.set(e, () => {
			if (!this.#t) {
				this.#t = !0;
				for (let e of this.#e) setTimeout(() => {
					e();
				}, 0);
				this.#e = [];
			}
		});
	}
	addListener(e) {
		u(!this.#t, "singal already cancelled", "UNSUPPORTED_OPERATION", { operation: "fetchCancelSignal.addCancelListener" }), this.#e.push(e);
	}
	get cancelled() {
		return this.#t;
	}
	checkSignal() {
		u(!this.cancelled, "cancelled", "CANCELLED", {});
	}
};
function De(e) {
	if (e == null) throw Error("missing signal; should not happen");
	return e.checkSignal(), e;
}
var Oe = class e {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	#o;
	#s;
	#c;
	#l;
	#u;
	#d;
	#f;
	#p;
	#m;
	get url() {
		return this.#a;
	}
	set url(e) {
		this.#a = String(e);
	}
	get body() {
		return this.#o == null ? null : new Uint8Array(this.#o);
	}
	set body(e) {
		if (e == null) this.#o = void 0, this.#s = void 0;
		else if (typeof e == "string") this.#o = H(e), this.#s = "text/plain";
		else if (e instanceof Uint8Array) this.#o = e, this.#s = "application/octet-stream";
		else if (typeof e == "object") this.#o = H(JSON.stringify(e)), this.#s = "application/json";
		else throw Error("invalid body");
	}
	hasBody() {
		return this.#o != null;
	}
	get method() {
		return this.#r ? this.#r : this.hasBody() ? "POST" : "GET";
	}
	set method(e) {
		e ??= "", this.#r = String(e).toUpperCase();
	}
	get headers() {
		let e = Object.assign({}, this.#n);
		return this.#c && (e.authorization = `Basic ${oe(H(this.#c))}`), this.allowGzip && (e["accept-encoding"] = "gzip"), e["content-type"] == null && this.#s && (e["content-type"] = this.#s), this.body && (e["content-length"] = String(this.body.length)), e;
	}
	getHeader(e) {
		return this.headers[e.toLowerCase()];
	}
	setHeader(e, t) {
		this.#n[String(e).toLowerCase()] = String(t);
	}
	clearHeaders() {
		this.#n = {};
	}
	[Symbol.iterator]() {
		let e = this.headers, t = Object.keys(e), n = 0;
		return { next: () => {
			if (n < t.length) {
				let r = t[n++];
				return {
					value: [r, e[r]],
					done: !1
				};
			}
			return {
				value: void 0,
				done: !0
			};
		} };
	}
	get credentials() {
		return this.#c || null;
	}
	setCredentials(e, t) {
		d(!e.match(/:/), "invalid basic authentication username", "username", "[REDACTED]"), this.#c = `${e}:${t}`;
	}
	get allowGzip() {
		return this.#t;
	}
	set allowGzip(e) {
		this.#t = !!e;
	}
	get allowInsecureAuthentication() {
		return !!this.#e;
	}
	set allowInsecureAuthentication(e) {
		this.#e = !!e;
	}
	get timeout() {
		return this.#i;
	}
	set timeout(e) {
		d(e >= 0, "timeout must be non-zero", "timeout", e), this.#i = e;
	}
	get preflightFunc() {
		return this.#l || null;
	}
	set preflightFunc(e) {
		this.#l = e;
	}
	get processFunc() {
		return this.#u || null;
	}
	set processFunc(e) {
		this.#u = e;
	}
	get retryFunc() {
		return this.#d || null;
	}
	set retryFunc(e) {
		this.#d = e;
	}
	get getUrlFunc() {
		return this.#m || ve;
	}
	set getUrlFunc(e) {
		this.#m = e;
	}
	constructor(e) {
		this.#a = String(e), this.#e = !1, this.#t = !0, this.#n = {}, this.#r = "", this.#i = 3e5, this.#p = {
			slotInterval: _e,
			maxAttempts: ge
		}, this.#m = null;
	}
	toString() {
		return `<FetchRequest method=${JSON.stringify(this.method)} url=${JSON.stringify(this.url)} headers=${JSON.stringify(this.headers)} body=${this.#o ? S(this.#o) : "null"}>`;
	}
	setThrottleParams(e) {
		e.slotInterval != null && (this.#p.slotInterval = e.slotInterval), e.maxAttempts != null && (this.#p.maxAttempts = e.maxAttempts);
	}
	async #h(e, t, n, r, i) {
		if (e >= this.#p.maxAttempts) return i.makeServerError("exceeded maximum retry limit");
		u(Ae() <= t, "timeout", "TIMEOUT", {
			operation: "request.send",
			reason: "timeout",
			request: r
		}), n > 0 && await Me(n);
		let a = this.clone(), o = (a.url.split(":")[0] || "").toLowerCase();
		if (o in we) {
			let e = await we[o](a.url, De(r.#f));
			if (e instanceof ke) {
				let t = e;
				if (this.processFunc) {
					De(r.#f);
					try {
						t = await this.processFunc(a, t);
					} catch (e) {
						(e.throttle == null || typeof e.stall != "number") && t.makeServerError("error in post-processing function", e).assertOk();
					}
				}
				return t;
			}
			a = e;
		}
		this.preflightFunc && (a = await this.preflightFunc(a));
		let s = await this.getUrlFunc(a, De(r.#f)), c = new ke(s.statusCode, s.statusMessage, s.headers, s.body, r);
		if ([
			301,
			302,
			307,
			308
		].indexOf(c.statusCode) >= 0) {
			try {
				let n = c.headers.location || "";
				return a.redirect(n).#h(e + 1, t, 0, r, c);
			} catch {}
			return c;
		}
		if (c.statusCode === 429 && (this.retryFunc == null || await this.retryFunc(a, c, e))) {
			let n = c.headers["retry-after"], i = this.#p.slotInterval * Math.trunc(Math.random() * 2 ** e);
			return typeof n == "string" && n.match(/^[1-9][0-9]*$/) && (i = parseInt(n)), a.clone().#h(e + 1, t, i, r, c);
		}
		if (this.processFunc) {
			De(r.#f);
			try {
				c = await this.processFunc(a, c);
			} catch (n) {
				(n.throttle == null || typeof n.stall != "number") && c.makeServerError("error in post-processing function", n).assertOk();
				let i = this.#p.slotInterval * Math.trunc(Math.random() * 2 ** e);
				return n.stall >= 0 && (i = n.stall), a.clone().#h(e + 1, t, i, r, c);
			}
		}
		return c;
	}
	send() {
		return u(this.#f == null, "request already sent", "UNSUPPORTED_OPERATION", { operation: "fetchRequest.send" }), this.#f = new Ee(this), this.#h(0, Ae() + this.timeout, 0, this, new ke(0, "", {}, null, this));
	}
	cancel() {
		u(this.#f != null, "request has not been sent", "UNSUPPORTED_OPERATION", { operation: "fetchRequest.cancel" });
		let e = Te.get(this);
		if (!e) throw Error("missing signal; should not happen");
		e();
	}
	redirect(t) {
		let n = this.url.split(":")[0].toLowerCase(), r = t.split(":")[0].toLowerCase();
		u((n !== "https" || r !== "http") && t.match(/^https?:/), "unsupported redirect", "UNSUPPORTED_OPERATION", { operation: `redirect(${this.method} ${JSON.stringify(this.url)} => ${JSON.stringify(t)})` });
		let i = new e(t);
		return i.method = this.method, i.allowGzip = this.allowGzip, i.timeout = this.timeout, i.#n = Object.assign({}, this.#n), this.#o && (i.#o = new Uint8Array(this.#o)), i.#s = this.#s, i;
	}
	clone() {
		let t = new e(this.url);
		return t.#r = this.#r, this.#o && (t.#o = this.#o), t.#s = this.#s, t.#n = Object.assign({}, this.#n), t.#c = this.#c, this.allowGzip && (t.allowGzip = !0), t.timeout = this.timeout, this.allowInsecureAuthentication && (t.allowInsecureAuthentication = !0), t.#l = this.#l, t.#u = this.#u, t.#d = this.#d, t.#p = Object.assign({}, this.#p), t.#m = this.#m, t;
	}
	static lockConfig() {
		xe = !0;
	}
	static getGateway(e) {
		return we[e.toLowerCase()] || null;
	}
	static registerGateway(e, t) {
		if (e = e.toLowerCase(), e === "http" || e === "https") throw Error(`cannot intercept ${e}; use registerGetUrl`);
		if (xe) throw Error("gateways locked");
		we[e] = t;
	}
	static registerGetUrl(e) {
		if (xe) throw Error("gateways locked");
		ve = e;
	}
	static createGetUrlFunc(e) {
		return he(e);
	}
	static createDataGateway() {
		return Se;
	}
	static createIpfsGatewayFunc(e) {
		return Ce(e);
	}
}, ke = class e {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	toString() {
		return `<FetchResponse status=${this.statusCode} body=${this.#r ? S(this.#r) : "null"}>`;
	}
	get statusCode() {
		return this.#e;
	}
	get statusMessage() {
		return this.#t;
	}
	get headers() {
		return Object.assign({}, this.#n);
	}
	get body() {
		return this.#r == null ? null : new Uint8Array(this.#r);
	}
	get bodyText() {
		try {
			return this.#r == null ? "" : me(this.#r);
		} catch {
			u(!1, "response body is not valid UTF-8 data", "UNSUPPORTED_OPERATION", {
				operation: "bodyText",
				info: { response: this }
			});
		}
	}
	get bodyJson() {
		try {
			return JSON.parse(this.bodyText);
		} catch {
			u(!1, "response body is not valid JSON", "UNSUPPORTED_OPERATION", {
				operation: "bodyJson",
				info: { response: this }
			});
		}
	}
	[Symbol.iterator]() {
		let e = this.headers, t = Object.keys(e), n = 0;
		return { next: () => {
			if (n < t.length) {
				let r = t[n++];
				return {
					value: [r, e[r]],
					done: !1
				};
			}
			return {
				value: void 0,
				done: !0
			};
		} };
	}
	constructor(e, t, n, r, i) {
		this.#e = e, this.#t = t, this.#n = Object.keys(n).reduce((e, t) => (e[t.toLowerCase()] = String(n[t]), e), {}), this.#r = r == null ? null : new Uint8Array(r), this.#i = i || null, this.#a = { message: "" };
	}
	makeServerError(t, n) {
		let r;
		t ? r = `CLIENT ESCALATED SERVER ERROR (${this.statusCode} ${this.statusMessage}; ${t})` : (t = `${this.statusCode} ${this.statusMessage}`, r = `CLIENT ESCALATED SERVER ERROR (${t})`);
		let i = new e(599, r, this.headers, this.body, this.#i || void 0);
		return i.#a = {
			message: t,
			error: n
		}, i;
	}
	throwThrottleError(e, t) {
		t == null ? t = -1 : d(Number.isInteger(t) && t >= 0, "invalid stall timeout", "stall", t);
		let n = Error(e || "throttling requests");
		throw a(n, {
			stall: t,
			throttle: !0
		}), n;
	}
	getHeader(e) {
		return this.headers[e.toLowerCase()];
	}
	hasBody() {
		return this.#r != null;
	}
	get request() {
		return this.#i;
	}
	ok() {
		return this.#a.message === "" && this.statusCode >= 200 && this.statusCode < 300;
	}
	assertOk() {
		if (this.ok()) return;
		let { message: e, error: t } = this.#a;
		e === "" && (e = `server response ${this.statusCode} ${this.statusMessage}`);
		let n = null;
		this.request && (n = this.request.url);
		let r = null;
		try {
			this.#r && (r = me(this.#r));
		} catch {}
		u(!1, e, "SERVER_ERROR", {
			request: this.request || "unknown request",
			response: this,
			error: t,
			info: {
				requestUrl: n,
				responseBody: r,
				responseStatus: `${this.statusCode} ${this.statusMessage}`
			}
		});
	}
};
function Ae() {
	return (/* @__PURE__ */ new Date()).getTime();
}
function je(e) {
	return H(e.replace(/%([0-9a-f][0-9a-f])/gi, (e, t) => String.fromCharCode(parseInt(t, 16))));
}
function Me(e) {
	return new Promise((t) => setTimeout(t, e));
}
for (var Ne = BigInt(-1), Pe = BigInt(0), Fe = BigInt(1), Ie = BigInt(5), Le = {}, Re = "0000"; Re.length < 80;) Re += Re;
function ze(e) {
	let t = Re;
	for (; t.length < e;) t += t;
	return BigInt("1" + t.substring(0, e));
}
function Be(e, t, n) {
	let r = BigInt(t.width);
	if (t.signed) {
		let t = Fe << r - Fe;
		u(n == null || e >= -t && e < t, "overflow", "NUMERIC_FAULT", {
			operation: n,
			fault: "overflow",
			value: e
		}), e = e > Pe ? M(P(e, r), r) : -M(P(-e, r), r);
	} else {
		let t = Fe << r;
		u(n == null || e >= 0 && e < t, "overflow", "NUMERIC_FAULT", {
			operation: n,
			fault: "overflow",
			value: e
		}), e = (e % t + t) % t & t - Fe;
	}
	return e;
}
function Ve(e) {
	typeof e == "number" && (e = `fixed128x${e}`);
	let t = !0, n = 128, r = 18;
	if (typeof e == "string") {
		if (e !== "fixed") {
			if (e === "ufixed") t = !1;
			else {
				let i = e.match(/^(u?)fixed([0-9]+)x([0-9]+)$/);
				d(i, "invalid fixed format", "format", e), t = i[1] !== "u", n = parseInt(i[2]), r = parseInt(i[3]);
			}
		}
	} else if (e) {
		let i = e, a = (e, t, n) => i[e] == null ? n : (d(typeof i[e] === t, "invalid fixed format (" + e + " not " + t + ")", "format." + e, i[e]), i[e]);
		t = a("signed", "boolean", t), n = a("width", "number", n), r = a("decimals", "number", r);
	}
	d(n % 8 == 0, "invalid FixedNumber width (not byte aligned)", "format.width", n), d(r <= 80, "invalid FixedNumber decimals (too large)", "format.decimals", r);
	let i = (t ? "" : "u") + "fixed" + String(n) + "x" + String(r);
	return {
		signed: t,
		width: n,
		decimals: r,
		name: i
	};
}
function He(e, t) {
	let n = "";
	e < Pe && (n = "-", e *= Ne);
	let r = e.toString();
	if (t === 0) return n + r;
	for (; r.length <= t;) r = Re + r;
	let i = r.length - t;
	for (r = r.substring(0, i) + "." + r.substring(i); r[0] === "0" && r[1] !== ".";) r = r.substring(1);
	for (; r[r.length - 1] === "0" && r[r.length - 2] !== ".";) r = r.substring(0, r.length - 1);
	return n + r;
}
var Ue = class e {
	format;
	#e;
	#t;
	#n;
	_value;
	constructor(e, t, n) {
		h(e, Le, "FixedNumber"), this.#t = t, this.#e = n;
		let r = He(t, n.decimals);
		a(this, {
			format: n.name,
			_value: r
		}), this.#n = ze(n.decimals);
	}
	get signed() {
		return this.#e.signed;
	}
	get width() {
		return this.#e.width;
	}
	get decimals() {
		return this.#e.decimals;
	}
	get value() {
		return this.#t;
	}
	#r(e) {
		d(this.format === e.format, "incompatible format; use fixedNumber.toFormat", "other", e);
	}
	#i(t, n) {
		return t = Be(t, this.#e, n), new e(Le, t, this.#e);
	}
	#a(e, t) {
		return this.#r(e), this.#i(this.#t + e.#t, t);
	}
	addUnsafe(e) {
		return this.#a(e);
	}
	add(e) {
		return this.#a(e, "add");
	}
	#o(e, t) {
		return this.#r(e), this.#i(this.#t - e.#t, t);
	}
	subUnsafe(e) {
		return this.#o(e);
	}
	sub(e) {
		return this.#o(e, "sub");
	}
	#s(e, t) {
		return this.#r(e), this.#i(this.#t * e.#t / this.#n, t);
	}
	mulUnsafe(e) {
		return this.#s(e);
	}
	mul(e) {
		return this.#s(e, "mul");
	}
	mulSignal(e) {
		this.#r(e);
		let t = this.#t * e.#t;
		return u(t % this.#n === Pe, "precision lost during signalling mul", "NUMERIC_FAULT", {
			operation: "mulSignal",
			fault: "underflow",
			value: this
		}), this.#i(t / this.#n, "mulSignal");
	}
	#c(e, t) {
		return u(e.#t !== Pe, "division by zero", "NUMERIC_FAULT", {
			operation: "div",
			fault: "divide-by-zero",
			value: this
		}), this.#r(e), this.#i(this.#t * this.#n / e.#t, t);
	}
	divUnsafe(e) {
		return this.#c(e);
	}
	div(e) {
		return this.#c(e, "div");
	}
	divSignal(e) {
		u(e.#t !== Pe, "division by zero", "NUMERIC_FAULT", {
			operation: "div",
			fault: "divide-by-zero",
			value: this
		}), this.#r(e);
		let t = this.#t * this.#n;
		return u(t % e.#t === Pe, "precision lost during signalling div", "NUMERIC_FAULT", {
			operation: "divSignal",
			fault: "underflow",
			value: this
		}), this.#i(t / e.#t, "divSignal");
	}
	cmp(e) {
		let t = this.value, n = e.value, r = this.decimals - e.decimals;
		return r > 0 ? n *= ze(r) : r < 0 && (t *= ze(-r)), t < n ? -1 : +(t > n);
	}
	eq(e) {
		return this.cmp(e) === 0;
	}
	lt(e) {
		return this.cmp(e) < 0;
	}
	lte(e) {
		return this.cmp(e) <= 0;
	}
	gt(e) {
		return this.cmp(e) > 0;
	}
	gte(e) {
		return this.cmp(e) >= 0;
	}
	floor() {
		let e = this.#t;
		return this.#t < Pe && (e -= this.#n - Fe), e = this.#t / this.#n * this.#n, this.#i(e, "floor");
	}
	ceiling() {
		let e = this.#t;
		return this.#t > Pe && (e += this.#n - Fe), e = this.#t / this.#n * this.#n, this.#i(e, "ceiling");
	}
	round(t) {
		if (t ??= 0, t >= this.decimals) return this;
		let n = this.decimals - t, r = Ie * ze(n - 1), i = this.value + r, a = ze(n);
		return i = i / a * a, Be(i, this.#e, "round"), new e(Le, i, this.#e);
	}
	isZero() {
		return this.#t === Pe;
	}
	isNegative() {
		return this.#t < Pe;
	}
	toString() {
		return this._value;
	}
	toUnsafeFloat() {
		return parseFloat(this.toString());
	}
	toFormat(t) {
		return e.fromString(this.toString(), t);
	}
	static fromValue(t, n, r) {
		let i = n == null ? 0 : z(n), a = Ve(r), o = F(t, "value"), s = i - a.decimals;
		if (s > 0) {
			let e = ze(s);
			u(o % e === Pe, "value loses precision for format", "NUMERIC_FAULT", {
				operation: "fromValue",
				fault: "underflow",
				value: t
			}), o /= e;
		} else s < 0 && (o *= ze(-s));
		return Be(o, a, "fromValue"), new e(Le, o, a);
	}
	static fromString(t, n) {
		let r = t.match(/^(-?)([0-9]*)\.?([0-9]*)$/);
		d(r && r[2].length + r[3].length > 0, "invalid FixedNumber string value", "value", t);
		let i = Ve(n), a = r[2] || "0", o = r[3] || "";
		for (; o.length < i.decimals;) o += Re;
		u(o.substring(i.decimals).match(/^0*$/), "too many decimals for format", "NUMERIC_FAULT", {
			operation: "fromString",
			fault: "underflow",
			value: t
		}), o = o.substring(0, i.decimals);
		let s = BigInt(r[1] + a + o);
		return Be(s, i, "fromString"), new e(Le, s, i);
	}
	static fromBytes(t, n) {
		let r = R(_(t, "value")), i = Ve(n);
		return i.signed && (r = M(r, i.width)), Be(r, i, "fromBytes"), new e(Le, r, i);
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/utils/rlp-decode.js
function We(e) {
	let t = e.toString(16);
	for (; t.length < 2;) t = "0" + t;
	return "0x" + t;
}
function Ge(e, t, n) {
	let r = 0;
	for (let i = 0; i < n; i++) r = r * 256 + e[t + i];
	return r;
}
function Ke(e, t, n, r) {
	let i = [];
	for (; n < t + 1 + r;) {
		let a = qe(e, n);
		i.push(a.result), n += a.consumed, u(n <= t + 1 + r, "child data too short", "BUFFER_OVERRUN", {
			buffer: e,
			length: r,
			offset: t
		});
	}
	return {
		consumed: 1 + r,
		result: i
	};
}
function qe(e, t) {
	u(e.length !== 0, "data too short", "BUFFER_OVERRUN", {
		buffer: e,
		length: 0,
		offset: 1
	});
	let n = (t) => {
		u(t <= e.length, "data short segment too short", "BUFFER_OVERRUN", {
			buffer: e,
			length: e.length,
			offset: t
		});
	};
	if (e[t] >= 248) {
		let r = e[t] - 247;
		n(t + 1 + r);
		let i = Ge(e, t + 1, r);
		return n(t + 1 + r + i), Ke(e, t, t + 1 + r, r + i);
	}
	if (e[t] >= 192) {
		let r = e[t] - 192;
		return n(t + 1 + r), Ke(e, t, t + 1, r);
	}
	if (e[t] >= 184) {
		let r = e[t] - 183;
		n(t + 1 + r);
		let i = Ge(e, t + 1, r);
		n(t + 1 + r + i);
		let a = S(e.slice(t + 1 + r, t + 1 + r + i));
		return {
			consumed: 1 + r + i,
			result: a
		};
	}
	if (e[t] >= 128) {
		let r = e[t] - 128;
		n(t + 1 + r);
		let i = S(e.slice(t + 1, t + 1 + r));
		return {
			consumed: 1 + r,
			result: i
		};
	}
	return {
		consumed: 1,
		result: We(e[t])
	};
}
function Je(e) {
	let t = _(e, "data"), n = qe(t, 0);
	return d(n.consumed === t.length, "unexpected junk after rlp payload", "data", e), n.result;
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/rlp-encode.js
function Ye(e) {
	let t = [];
	for (; e;) t.unshift(e & 255), e >>= 8;
	return t;
}
function Xe(e) {
	if (Array.isArray(e)) {
		let t = [];
		if (e.forEach(function(e) {
			t = t.concat(Xe(e));
		}), t.length <= 55) return t.unshift(192 + t.length), t;
		let n = Ye(t.length);
		return n.unshift(247 + n.length), n.concat(t);
	}
	let t = Array.prototype.slice.call(_(e, "object"));
	if (t.length === 1 && t[0] <= 127) return t;
	if (t.length <= 55) return t.unshift(128 + t.length), t;
	let n = Ye(t.length);
	return n.unshift(183 + n.length), n.concat(t);
}
var Ze = "0123456789abcdef";
function Qe(e) {
	let t = "0x";
	for (let n of Xe(e)) t += Ze[n >> 4], t += Ze[n & 15];
	return t;
}
//#endregion
//#region node_modules/ethers/lib.esm/utils/units.js
var $e = [
	"wei",
	"kwei",
	"mwei",
	"gwei",
	"szabo",
	"finney",
	"ether"
];
function et(e, t) {
	let n = 18;
	if (typeof t == "string") {
		let e = $e.indexOf(t);
		d(e >= 0, "invalid unit", "unit", t), n = 3 * e;
	} else t != null && (n = z(t, "unit"));
	return Ue.fromValue(e, n, {
		decimals: n,
		width: 512
	}).toString();
}
function tt(e, t) {
	d(typeof e == "string", "value must be a string", "value", e);
	let n = 18;
	if (typeof t == "string") {
		let e = $e.indexOf(t);
		d(e >= 0, "invalid unit", "unit", t), n = 3 * e;
	} else t != null && (n = z(t, "unit"));
	return Ue.fromString(e, {
		decimals: n,
		width: 512
	}).value;
}
function nt(e) {
	return et(e, 18);
}
var rt = /* @__PURE__ */ new Uint8Array(32), it = ["then"], at = {}, ot = /* @__PURE__ */ new WeakMap();
function st(e) {
	return ot.get(e);
}
function ct(e, t) {
	ot.set(e, t);
}
function lt(e, t) {
	let n = /* @__PURE__ */ Error(`deferred error during ABI decoding triggered accessing ${e}`);
	throw n.error = t, n;
}
function ut(e, t, n) {
	return e.indexOf(null) >= 0 ? t.map((e, t) => e instanceof dt ? ut(st(e), e, n) : e) : e.reduce((e, r, i) => {
		let a = t.getValue(r);
		return r in e || (n && a instanceof dt && (a = ut(st(a), a, n)), e[r] = a), e;
	}, {});
}
var dt = class e extends Array {
	#e;
	constructor(...e) {
		let t = e[0], n = e[1], r = (e[2] || []).slice(), i = !0;
		t !== at && (n = e, r = [], i = !1), super(n.length), n.forEach((e, t) => {
			this[t] = e;
		});
		let a = r.reduce((e, t) => (typeof t == "string" && e.set(t, (e.get(t) || 0) + 1), e), /* @__PURE__ */ new Map());
		if (ct(this, Object.freeze(n.map((e, t) => {
			let n = r[t];
			return n != null && a.get(n) === 1 ? n : null;
		}))), this.#e = [], this.#e ?? this.#e, !i) return;
		Object.freeze(this);
		let o = new Proxy(this, { get: (e, t, n) => {
			if (typeof t == "string") {
				if (t.match(/^[0-9]+$/)) {
					let n = z(t, "%index");
					if (n < 0 || n >= this.length) throw RangeError("out of result range");
					let r = e[n];
					return r instanceof Error && lt(`index ${n}`, r), r;
				}
				if (it.indexOf(t) >= 0) return Reflect.get(e, t, n);
				let r = e[t];
				if (r instanceof Function) return function(...t) {
					return r.apply(this === n ? e : this, t);
				};
				if (!(t in e)) return e.getValue.apply(this === n ? e : this, [t]);
			}
			return Reflect.get(e, t, n);
		} });
		return ct(o, st(this)), o;
	}
	toArray(t) {
		let n = [];
		return this.forEach((r, i) => {
			r instanceof Error && lt(`index ${i}`, r), t && r instanceof e && (r = r.toArray(t)), n.push(r);
		}), n;
	}
	toObject(e) {
		let t = st(this);
		return t.reduce((n, r, i) => (u(r != null, `value at index ${i} unnamed`, "UNSUPPORTED_OPERATION", { operation: "toObject()" }), ut(t, this, e)), {});
	}
	slice(t, n) {
		t ??= 0, t < 0 && (t += this.length, t < 0 && (t = 0)), n ??= this.length, n < 0 && (n += this.length, n < 0 && (n = 0)), n > this.length && (n = this.length);
		let r = st(this), i = [], a = [];
		for (let e = t; e < n; e++) i.push(this[e]), a.push(r[e]);
		return new e(at, i, a);
	}
	filter(t, n) {
		let r = st(this), i = [], a = [];
		for (let e = 0; e < this.length; e++) {
			let o = this[e];
			o instanceof Error && lt(`index ${e}`, o), t.call(n, o, e, this) && (i.push(o), a.push(r[e]));
		}
		return new e(at, i, a);
	}
	map(e, t) {
		let n = [];
		for (let r = 0; r < this.length; r++) {
			let i = this[r];
			i instanceof Error && lt(`index ${r}`, i), n.push(e.call(t, i, r, this));
		}
		return n;
	}
	getValue(e) {
		let t = st(this).indexOf(e);
		if (t === -1) return;
		let n = this[t];
		return n instanceof Error && lt(`property ${JSON.stringify(e)}`, n.error), n;
	}
	static fromItems(t, n) {
		return new e(at, t, n);
	}
};
function ft(e) {
	let t = B(e);
	return u(t.length <= 32, "value out-of-bounds", "BUFFER_OVERRUN", {
		buffer: t,
		length: 32,
		offset: t.length
	}), t.length !== 32 && (t = v(C([rt.slice(t.length % 32), t]))), t;
}
var pt = class {
	name;
	type;
	localName;
	dynamic;
	constructor(e, t, n, r) {
		a(this, {
			name: e,
			type: t,
			localName: n,
			dynamic: r
		}, {
			name: "string",
			type: "string",
			localName: "string",
			dynamic: "boolean"
		});
	}
	_throwError(e, t) {
		d(!1, e, this.localName, t);
	}
}, mt = class {
	#e;
	#t;
	constructor() {
		this.#e = [], this.#t = 0;
	}
	get data() {
		return C(this.#e);
	}
	get length() {
		return this.#t;
	}
	#n(e) {
		return this.#e.push(e), this.#t += e.length, e.length;
	}
	appendWriter(e) {
		return this.#n(v(e.data));
	}
	writeBytes(e) {
		let t = v(e), n = t.length % 32;
		return n && (t = v(C([t, rt.slice(n)]))), this.#n(t);
	}
	writeValue(e) {
		return this.#n(ft(e));
	}
	writeUpdatableValue() {
		let e = this.#e.length;
		return this.#e.push(rt), this.#t += 32, (t) => {
			this.#e[e] = ft(t);
		};
	}
}, ht = class e {
	allowLoose;
	#e;
	#t;
	#n;
	#r;
	#i;
	constructor(e, t, n) {
		a(this, { allowLoose: !!t }), this.#e = v(e), this.#n = 0, this.#r = null, this.#i = n ?? 1024, this.#t = 0;
	}
	get data() {
		return S(this.#e);
	}
	get dataLength() {
		return this.#e.length;
	}
	get consumed() {
		return this.#t;
	}
	get bytes() {
		return new Uint8Array(this.#e);
	}
	#a(e) {
		if (this.#r) return this.#r.#a(e);
		this.#n += e, u(this.#i < 1 || this.#n <= this.#i * this.dataLength, `compressed ABI data exceeds inflation ratio of ${this.#i} ( see: https:/\/github.com/ethers-io/ethers.js/issues/4537 )`, "BUFFER_OVERRUN", {
			buffer: v(this.#e),
			offset: this.#t,
			length: e,
			info: {
				bytesRead: this.#n,
				dataLength: this.dataLength
			}
		});
	}
	#o(e, t, n) {
		let r = Math.ceil(t / 32) * 32;
		return this.#t + r > this.#e.length && (this.allowLoose && n && this.#t + t <= this.#e.length ? r = t : u(!1, "data out-of-bounds", "BUFFER_OVERRUN", {
			buffer: v(this.#e),
			length: this.#e.length,
			offset: this.#t + r
		})), this.#e.slice(this.#t, this.#t + r);
	}
	subReader(t) {
		let n = new e(this.#e.slice(this.#t + t), this.allowLoose, this.#i);
		return n.#r = this, n;
	}
	readBytes(e, t) {
		let n = this.#o(0, e, !!t);
		return this.#a(e), this.#t += n.length, n.slice(0, e);
	}
	readValue() {
		return R(this.readBytes(32));
	}
	readIndex() {
		return ee(this.readBytes(32));
	}
};
//#endregion
//#region node_modules/@noble/hashes/esm/_assert.js
function gt(e) {
	if (!Number.isSafeInteger(e) || e < 0) throw Error(`Wrong positive integer: ${e}`);
}
function _t(e, ...t) {
	if (!(e instanceof Uint8Array)) throw Error("Expected Uint8Array");
	if (t.length > 0 && !t.includes(e.length)) throw Error(`Expected Uint8Array of length ${t}, not of length=${e.length}`);
}
function vt(e) {
	if (typeof e != "function" || typeof e.create != "function") throw Error("Hash should be wrapped by utils.wrapConstructor");
	gt(e.outputLen), gt(e.blockLen);
}
function yt(e, t = !0) {
	if (e.destroyed) throw Error("Hash instance has been destroyed");
	if (t && e.finished) throw Error("Hash#digest() has already been called");
}
function bt(e, t) {
	_t(e);
	let n = t.outputLen;
	if (e.length < n) throw Error(`digestInto() expects output buffer of length at least ${n}`);
}
//#endregion
//#region node_modules/@noble/hashes/esm/crypto.js
var xt = typeof globalThis == "object" && "crypto" in globalThis ? globalThis.crypto : void 0, St = (e) => e instanceof Uint8Array, Ct = (e) => new Uint32Array(e.buffer, e.byteOffset, Math.floor(e.byteLength / 4)), wt = (e) => new DataView(e.buffer, e.byteOffset, e.byteLength), Tt = (e, t) => e << 32 - t | e >>> t;
if (new Uint8Array(new Uint32Array([287454020]).buffer)[0] !== 68) throw Error("Non little-endian hardware is not supported");
var Et = async () => {};
async function Dt(e, t, n) {
	let r = Date.now();
	for (let i = 0; i < e; i++) {
		n(i);
		let e = Date.now() - r;
		e >= 0 && e < t || (await /* @__PURE__ */ Et(), r += e);
	}
}
function Ot(e) {
	if (typeof e != "string") throw Error(`utf8ToBytes expected string, got ${typeof e}`);
	return new Uint8Array(new TextEncoder().encode(e));
}
function kt(e) {
	if (typeof e == "string" && (e = Ot(e)), !St(e)) throw Error(`expected Uint8Array, got ${typeof e}`);
	return e;
}
function At(...e) {
	let t = new Uint8Array(e.reduce((e, t) => e + t.length, 0)), n = 0;
	return e.forEach((e) => {
		if (!St(e)) throw Error("Uint8Array expected");
		t.set(e, n), n += e.length;
	}), t;
}
var jt = class {
	clone() {
		return this._cloneInto();
	}
}, Mt = {}.toString;
function Nt(e, t) {
	if (t !== void 0 && Mt.call(t) !== "[object Object]") throw Error("Options should be object or undefined");
	return Object.assign(e, t);
}
function Pt(e) {
	let t = (t) => e().update(kt(t)).digest(), n = e();
	return t.outputLen = n.outputLen, t.blockLen = n.blockLen, t.create = () => e(), t;
}
function Ft(e = 32) {
	if (xt && typeof xt.getRandomValues == "function") return xt.getRandomValues(new Uint8Array(e));
	throw Error("crypto.getRandomValues must be defined");
}
//#endregion
//#region node_modules/@noble/hashes/esm/hmac.js
var It = class extends jt {
	constructor(e, t) {
		super(), this.finished = !1, this.destroyed = !1, vt(e);
		let n = kt(t);
		if (this.iHash = e.create(), typeof this.iHash.update != "function") throw Error("Expected instance of class which extends utils.Hash");
		this.blockLen = this.iHash.blockLen, this.outputLen = this.iHash.outputLen;
		let r = this.blockLen, i = new Uint8Array(r);
		i.set(n.length > r ? e.create().update(n).digest() : n);
		for (let e = 0; e < i.length; e++) i[e] ^= 54;
		this.iHash.update(i), this.oHash = e.create();
		for (let e = 0; e < i.length; e++) i[e] ^= 106;
		this.oHash.update(i), i.fill(0);
	}
	update(e) {
		return yt(this), this.iHash.update(e), this;
	}
	digestInto(e) {
		yt(this), _t(e, this.outputLen), this.finished = !0, this.iHash.digestInto(e), this.oHash.update(e), this.oHash.digestInto(e), this.destroy();
	}
	digest() {
		let e = new Uint8Array(this.oHash.outputLen);
		return this.digestInto(e), e;
	}
	_cloneInto(e) {
		e ||= Object.create(Object.getPrototypeOf(this), {});
		let { oHash: t, iHash: n, finished: r, destroyed: i, blockLen: a, outputLen: o } = this;
		return e = e, e.finished = r, e.destroyed = i, e.blockLen = a, e.outputLen = o, e.oHash = t._cloneInto(e.oHash), e.iHash = n._cloneInto(e.iHash), e;
	}
	destroy() {
		this.destroyed = !0, this.oHash.destroy(), this.iHash.destroy();
	}
}, Lt = (e, t, n) => new It(e, t).update(n).digest();
Lt.create = (e, t) => new It(e, t);
//#endregion
//#region node_modules/@noble/hashes/esm/pbkdf2.js
function Rt(e, t, n, r) {
	vt(e);
	let { c: i, dkLen: a, asyncTick: o } = Nt({
		dkLen: 32,
		asyncTick: 10
	}, r);
	if (gt(i), gt(a), gt(o), i < 1) throw Error("PBKDF2: iterations (c) should be >= 1");
	let s = kt(t), c = kt(n), l = new Uint8Array(a), u = Lt.create(e, s);
	return {
		c: i,
		dkLen: a,
		asyncTick: o,
		DK: l,
		PRF: u,
		PRFSalt: u._cloneInto().update(c)
	};
}
function zt(e, t, n, r, i) {
	return e.destroy(), t.destroy(), r && r.destroy(), i.fill(0), n;
}
function Bt(e, t, n, r) {
	let { c: i, dkLen: a, DK: o, PRF: s, PRFSalt: c } = Rt(e, t, n, r), l, u = /* @__PURE__ */ new Uint8Array(4), d = wt(u), f = new Uint8Array(s.outputLen);
	for (let e = 1, t = 0; t < a; e++, t += s.outputLen) {
		let n = o.subarray(t, t + s.outputLen);
		d.setInt32(0, e, !1), (l = c._cloneInto(l)).update(u).digestInto(f), n.set(f.subarray(0, n.length));
		for (let e = 1; e < i; e++) {
			s._cloneInto(l).update(f).digestInto(f);
			for (let e = 0; e < n.length; e++) n[e] ^= f[e];
		}
	}
	return zt(s, c, o, l, f);
}
//#endregion
//#region node_modules/@noble/hashes/esm/_sha2.js
function Vt(e, t, n, r) {
	if (typeof e.setBigUint64 == "function") return e.setBigUint64(t, n, r);
	let i = BigInt(32), a = BigInt(4294967295), o = Number(n >> i & a), s = Number(n & a), c = r ? 4 : 0, l = r ? 0 : 4;
	e.setUint32(t + c, o, r), e.setUint32(t + l, s, r);
}
var Ht = class extends jt {
	constructor(e, t, n, r) {
		super(), this.blockLen = e, this.outputLen = t, this.padOffset = n, this.isLE = r, this.finished = !1, this.length = 0, this.pos = 0, this.destroyed = !1, this.buffer = new Uint8Array(e), this.view = wt(this.buffer);
	}
	update(e) {
		yt(this);
		let { view: t, buffer: n, blockLen: r } = this;
		e = kt(e);
		let i = e.length;
		for (let a = 0; a < i;) {
			let o = Math.min(r - this.pos, i - a);
			if (o === r) {
				let t = wt(e);
				for (; r <= i - a; a += r) this.process(t, a);
				continue;
			}
			n.set(e.subarray(a, a + o), this.pos), this.pos += o, a += o, this.pos === r && (this.process(t, 0), this.pos = 0);
		}
		return this.length += e.length, this.roundClean(), this;
	}
	digestInto(e) {
		yt(this), bt(e, this), this.finished = !0;
		let { buffer: t, view: n, blockLen: r, isLE: i } = this, { pos: a } = this;
		t[a++] = 128, this.buffer.subarray(a).fill(0), this.padOffset > r - a && (this.process(n, 0), a = 0);
		for (let e = a; e < r; e++) t[e] = 0;
		Vt(n, r - 8, BigInt(this.length * 8), i), this.process(n, 0);
		let o = wt(e), s = this.outputLen;
		if (s % 4) throw Error("_sha2: outputLen should be aligned to 32bit");
		let c = s / 4, l = this.get();
		if (c > l.length) throw Error("_sha2: outputLen bigger than state");
		for (let e = 0; e < c; e++) o.setUint32(4 * e, l[e], i);
	}
	digest() {
		let { buffer: e, outputLen: t } = this;
		this.digestInto(e);
		let n = e.slice(0, t);
		return this.destroy(), n;
	}
	_cloneInto(e) {
		e ||= new this.constructor(), e.set(...this.get());
		let { blockLen: t, buffer: n, length: r, finished: i, destroyed: a, pos: o } = this;
		return e.length = r, e.pos = o, e.finished = i, e.destroyed = a, r % t && e.buffer.set(n), e;
	}
}, Ut = (e, t, n) => e & t ^ ~e & n, Wt = (e, t, n) => e & t ^ e & n ^ t & n, Gt = /* @__PURE__ */ new Uint32Array([
	1116352408,
	1899447441,
	3049323471,
	3921009573,
	961987163,
	1508970993,
	2453635748,
	2870763221,
	3624381080,
	310598401,
	607225278,
	1426881987,
	1925078388,
	2162078206,
	2614888103,
	3248222580,
	3835390401,
	4022224774,
	264347078,
	604807628,
	770255983,
	1249150122,
	1555081692,
	1996064986,
	2554220882,
	2821834349,
	2952996808,
	3210313671,
	3336571891,
	3584528711,
	113926993,
	338241895,
	666307205,
	773529912,
	1294757372,
	1396182291,
	1695183700,
	1986661051,
	2177026350,
	2456956037,
	2730485921,
	2820302411,
	3259730800,
	3345764771,
	3516065817,
	3600352804,
	4094571909,
	275423344,
	430227734,
	506948616,
	659060556,
	883997877,
	958139571,
	1322822218,
	1537002063,
	1747873779,
	1955562222,
	2024104815,
	2227730452,
	2361852424,
	2428436474,
	2756734187,
	3204031479,
	3329325298
]), Kt = /* @__PURE__ */ new Uint32Array([
	1779033703,
	3144134277,
	1013904242,
	2773480762,
	1359893119,
	2600822924,
	528734635,
	1541459225
]), qt = /* @__PURE__ */ new Uint32Array(64), Jt = class extends Ht {
	constructor() {
		super(64, 32, 8, !1), this.A = Kt[0] | 0, this.B = Kt[1] | 0, this.C = Kt[2] | 0, this.D = Kt[3] | 0, this.E = Kt[4] | 0, this.F = Kt[5] | 0, this.G = Kt[6] | 0, this.H = Kt[7] | 0;
	}
	get() {
		let { A: e, B: t, C: n, D: r, E: i, F: a, G: o, H: s } = this;
		return [
			e,
			t,
			n,
			r,
			i,
			a,
			o,
			s
		];
	}
	set(e, t, n, r, i, a, o, s) {
		this.A = e | 0, this.B = t | 0, this.C = n | 0, this.D = r | 0, this.E = i | 0, this.F = a | 0, this.G = o | 0, this.H = s | 0;
	}
	process(e, t) {
		for (let n = 0; n < 16; n++, t += 4) qt[n] = e.getUint32(t, !1);
		for (let e = 16; e < 64; e++) {
			let t = qt[e - 15], n = qt[e - 2], r = Tt(t, 7) ^ Tt(t, 18) ^ t >>> 3, i = Tt(n, 17) ^ Tt(n, 19) ^ n >>> 10;
			qt[e] = i + qt[e - 7] + r + qt[e - 16] | 0;
		}
		let { A: n, B: r, C: i, D: a, E: o, F: s, G: c, H: l } = this;
		for (let e = 0; e < 64; e++) {
			let t = Tt(o, 6) ^ Tt(o, 11) ^ Tt(o, 25), u = l + t + Ut(o, s, c) + Gt[e] + qt[e] | 0, d = (Tt(n, 2) ^ Tt(n, 13) ^ Tt(n, 22)) + Wt(n, r, i) | 0;
			l = c, c = s, s = o, o = a + u | 0, a = i, i = r, r = n, n = u + d | 0;
		}
		n = n + this.A | 0, r = r + this.B | 0, i = i + this.C | 0, a = a + this.D | 0, o = o + this.E | 0, s = s + this.F | 0, c = c + this.G | 0, l = l + this.H | 0, this.set(n, r, i, a, o, s, c, l);
	}
	roundClean() {
		qt.fill(0);
	}
	destroy() {
		this.set(0, 0, 0, 0, 0, 0, 0, 0), this.buffer.fill(0);
	}
}, Yt = /* @__PURE__ */ Pt(() => new Jt()), Xt = /* @__PURE__ */ BigInt(2 ** 32 - 1), Zt = /* @__PURE__ */ BigInt(32);
function Qt(e, t = !1) {
	return t ? {
		h: Number(e & Xt),
		l: Number(e >> Zt & Xt)
	} : {
		h: Number(e >> Zt & Xt) | 0,
		l: Number(e & Xt) | 0
	};
}
function $t(e, t = !1) {
	let n = new Uint32Array(e.length), r = new Uint32Array(e.length);
	for (let i = 0; i < e.length; i++) {
		let { h: a, l: o } = Qt(e[i], t);
		[n[i], r[i]] = [a, o];
	}
	return [n, r];
}
var en = (e, t) => BigInt(e >>> 0) << Zt | BigInt(t >>> 0), tn = (e, t, n) => e >>> n, nn = (e, t, n) => e << 32 - n | t >>> n, rn = (e, t, n) => e >>> n | t << 32 - n, an = (e, t, n) => e << 32 - n | t >>> n, on = (e, t, n) => e << 64 - n | t >>> n - 32, sn = (e, t, n) => e >>> n - 32 | t << 64 - n, cn = (e, t) => t, ln = (e, t) => e, un = (e, t, n) => e << n | t >>> 32 - n, dn = (e, t, n) => t << n | e >>> 32 - n, fn = (e, t, n) => t << n - 32 | e >>> 64 - n, pn = (e, t, n) => e << n - 32 | t >>> 64 - n;
function mn(e, t, n, r) {
	let i = (t >>> 0) + (r >>> 0);
	return {
		h: e + n + (i / 2 ** 32 | 0) | 0,
		l: i | 0
	};
}
var U = {
	fromBig: Qt,
	split: $t,
	toBig: en,
	shrSH: tn,
	shrSL: nn,
	rotrSH: rn,
	rotrSL: an,
	rotrBH: on,
	rotrBL: sn,
	rotr32H: cn,
	rotr32L: ln,
	rotlSH: un,
	rotlSL: dn,
	rotlBH: fn,
	rotlBL: pn,
	add: mn,
	add3L: (e, t, n) => (e >>> 0) + (t >>> 0) + (n >>> 0),
	add3H: (e, t, n, r) => t + n + r + (e / 2 ** 32 | 0) | 0,
	add4L: (e, t, n, r) => (e >>> 0) + (t >>> 0) + (n >>> 0) + (r >>> 0),
	add4H: (e, t, n, r, i) => t + n + r + i + (e / 2 ** 32 | 0) | 0,
	add5H: (e, t, n, r, i, a) => t + n + r + i + a + (e / 2 ** 32 | 0) | 0,
	add5L: (e, t, n, r, i) => (e >>> 0) + (t >>> 0) + (n >>> 0) + (r >>> 0) + (i >>> 0)
}, [hn, gn] = /* @__PURE__ */ U.split((/* @__PURE__ */ "0x428a2f98d728ae22.0x7137449123ef65cd.0xb5c0fbcfec4d3b2f.0xe9b5dba58189dbbc.0x3956c25bf348b538.0x59f111f1b605d019.0x923f82a4af194f9b.0xab1c5ed5da6d8118.0xd807aa98a3030242.0x12835b0145706fbe.0x243185be4ee4b28c.0x550c7dc3d5ffb4e2.0x72be5d74f27b896f.0x80deb1fe3b1696b1.0x9bdc06a725c71235.0xc19bf174cf692694.0xe49b69c19ef14ad2.0xefbe4786384f25e3.0x0fc19dc68b8cd5b5.0x240ca1cc77ac9c65.0x2de92c6f592b0275.0x4a7484aa6ea6e483.0x5cb0a9dcbd41fbd4.0x76f988da831153b5.0x983e5152ee66dfab.0xa831c66d2db43210.0xb00327c898fb213f.0xbf597fc7beef0ee4.0xc6e00bf33da88fc2.0xd5a79147930aa725.0x06ca6351e003826f.0x142929670a0e6e70.0x27b70a8546d22ffc.0x2e1b21385c26c926.0x4d2c6dfc5ac42aed.0x53380d139d95b3df.0x650a73548baf63de.0x766a0abb3c77b2a8.0x81c2c92e47edaee6.0x92722c851482353b.0xa2bfe8a14cf10364.0xa81a664bbc423001.0xc24b8b70d0f89791.0xc76c51a30654be30.0xd192e819d6ef5218.0xd69906245565a910.0xf40e35855771202a.0x106aa07032bbd1b8.0x19a4c116b8d2d0c8.0x1e376c085141ab53.0x2748774cdf8eeb99.0x34b0bcb5e19b48a8.0x391c0cb3c5c95a63.0x4ed8aa4ae3418acb.0x5b9cca4f7763e373.0x682e6ff3d6b2b8a3.0x748f82ee5defb2fc.0x78a5636f43172f60.0x84c87814a1f0ab72.0x8cc702081a6439ec.0x90befffa23631e28.0xa4506cebde82bde9.0xbef9a3f7b2c67915.0xc67178f2e372532b.0xca273eceea26619c.0xd186b8c721c0c207.0xeada7dd6cde0eb1e.0xf57d4f7fee6ed178.0x06f067aa72176fba.0x0a637dc5a2c898a6.0x113f9804bef90dae.0x1b710b35131c471b.0x28db77f523047d84.0x32caab7b40c72493.0x3c9ebe0a15c9bebc.0x431d67c49c100d4c.0x4cc5d4becb3e42b6.0x597f299cfc657e2a.0x5fcb6fab3ad6faec.0x6c44198c4a475817".split(".")).map((e) => BigInt(e))), _n = /* @__PURE__ */ new Uint32Array(80), vn = /* @__PURE__ */ new Uint32Array(80), yn = class extends Ht {
	constructor() {
		super(128, 64, 16, !1), this.Ah = 1779033703, this.Al = -205731576, this.Bh = -1150833019, this.Bl = -2067093701, this.Ch = 1013904242, this.Cl = -23791573, this.Dh = -1521486534, this.Dl = 1595750129, this.Eh = 1359893119, this.El = -1377402159, this.Fh = -1694144372, this.Fl = 725511199, this.Gh = 528734635, this.Gl = -79577749, this.Hh = 1541459225, this.Hl = 327033209;
	}
	get() {
		let { Ah: e, Al: t, Bh: n, Bl: r, Ch: i, Cl: a, Dh: o, Dl: s, Eh: c, El: l, Fh: u, Fl: d, Gh: f, Gl: p, Hh: m, Hl: h } = this;
		return [
			e,
			t,
			n,
			r,
			i,
			a,
			o,
			s,
			c,
			l,
			u,
			d,
			f,
			p,
			m,
			h
		];
	}
	set(e, t, n, r, i, a, o, s, c, l, u, d, f, p, m, h) {
		this.Ah = e | 0, this.Al = t | 0, this.Bh = n | 0, this.Bl = r | 0, this.Ch = i | 0, this.Cl = a | 0, this.Dh = o | 0, this.Dl = s | 0, this.Eh = c | 0, this.El = l | 0, this.Fh = u | 0, this.Fl = d | 0, this.Gh = f | 0, this.Gl = p | 0, this.Hh = m | 0, this.Hl = h | 0;
	}
	process(e, t) {
		for (let n = 0; n < 16; n++, t += 4) _n[n] = e.getUint32(t), vn[n] = e.getUint32(t += 4);
		for (let e = 16; e < 80; e++) {
			let t = _n[e - 15] | 0, n = vn[e - 15] | 0, r = U.rotrSH(t, n, 1) ^ U.rotrSH(t, n, 8) ^ U.shrSH(t, n, 7), i = U.rotrSL(t, n, 1) ^ U.rotrSL(t, n, 8) ^ U.shrSL(t, n, 7), a = _n[e - 2] | 0, o = vn[e - 2] | 0, s = U.rotrSH(a, o, 19) ^ U.rotrBH(a, o, 61) ^ U.shrSH(a, o, 6), c = U.rotrSL(a, o, 19) ^ U.rotrBL(a, o, 61) ^ U.shrSL(a, o, 6), l = U.add4L(i, c, vn[e - 7], vn[e - 16]), u = U.add4H(l, r, s, _n[e - 7], _n[e - 16]);
			_n[e] = u | 0, vn[e] = l | 0;
		}
		let { Ah: n, Al: r, Bh: i, Bl: a, Ch: o, Cl: s, Dh: c, Dl: l, Eh: u, El: d, Fh: f, Fl: p, Gh: m, Gl: h, Hh: g, Hl: _ } = this;
		for (let e = 0; e < 80; e++) {
			let t = U.rotrSH(u, d, 14) ^ U.rotrSH(u, d, 18) ^ U.rotrBH(u, d, 41), v = U.rotrSL(u, d, 14) ^ U.rotrSL(u, d, 18) ^ U.rotrBL(u, d, 41), y = u & f ^ ~u & m, b = d & p ^ ~d & h, x = U.add5L(_, v, b, gn[e], vn[e]), S = U.add5H(x, g, t, y, hn[e], _n[e]), C = x | 0, w = U.rotrSH(n, r, 28) ^ U.rotrBH(n, r, 34) ^ U.rotrBH(n, r, 39), T = U.rotrSL(n, r, 28) ^ U.rotrBL(n, r, 34) ^ U.rotrBL(n, r, 39), E = n & i ^ n & o ^ i & o, D = r & a ^ r & s ^ a & s;
			g = m | 0, _ = h | 0, m = f | 0, h = p | 0, f = u | 0, p = d | 0, {h: u, l: d} = U.add(c | 0, l | 0, S | 0, C | 0), c = o | 0, l = s | 0, o = i | 0, s = a | 0, i = n | 0, a = r | 0;
			let O = U.add3L(C, T, D);
			n = U.add3H(O, S, w, E), r = O | 0;
		}
		({h: n, l: r} = U.add(this.Ah | 0, this.Al | 0, n | 0, r | 0)), {h: i, l: a} = U.add(this.Bh | 0, this.Bl | 0, i | 0, a | 0), {h: o, l: s} = U.add(this.Ch | 0, this.Cl | 0, o | 0, s | 0), {h: c, l: l} = U.add(this.Dh | 0, this.Dl | 0, c | 0, l | 0), {h: u, l: d} = U.add(this.Eh | 0, this.El | 0, u | 0, d | 0), {h: f, l: p} = U.add(this.Fh | 0, this.Fl | 0, f | 0, p | 0), {h: m, l: h} = U.add(this.Gh | 0, this.Gl | 0, m | 0, h | 0), {h: g, l: _} = U.add(this.Hh | 0, this.Hl | 0, g | 0, _ | 0), this.set(n, r, i, a, o, s, c, l, u, d, f, p, m, h, g, _);
	}
	roundClean() {
		_n.fill(0), vn.fill(0);
	}
	destroy() {
		this.buffer.fill(0), this.set(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0);
	}
}, bn = /* @__PURE__ */ Pt(() => new yn());
//#endregion
//#region node_modules/ethers/lib.esm/crypto/crypto-browser.js
function xn() {
	if (typeof self < "u") return self;
	if (typeof window < "u") return window;
	if (typeof global < "u") return global;
	throw Error("unable to locate global object");
}
var Sn = xn(), Cn = Sn.crypto || Sn.msCrypto;
function wn(e) {
	switch (e) {
		case "sha256": return Yt.create();
		case "sha512": return bn.create();
	}
	d(!1, "invalid hashing algorithm name", "algorithm", e);
}
function Tn(e, t, n, r, i) {
	let a = {
		sha256: Yt,
		sha512: bn
	}[i];
	return d(a != null, "invalid pbkdf2 algorithm", "algorithm", i), Bt(a, e, t, {
		c: n,
		dkLen: r
	});
}
function En(e) {
	u(Cn != null, "platform does not support secure random numbers", "UNSUPPORTED_OPERATION", { operation: "randomBytes" }), d(Number.isInteger(e) && e > 0 && e <= 1024, "invalid length", "length", e);
	let t = new Uint8Array(e);
	return Cn.getRandomValues(t), t;
}
//#endregion
//#region node_modules/@noble/hashes/esm/sha3.js
var [Dn, On, kn] = [
	[],
	[],
	[]
], An = /* @__PURE__ */ BigInt(0), jn = /* @__PURE__ */ BigInt(1), Mn = /* @__PURE__ */ BigInt(2), Nn = /* @__PURE__ */ BigInt(7), Pn = /* @__PURE__ */ BigInt(256), Fn = /* @__PURE__ */ BigInt(113);
for (let e = 0, t = jn, n = 1, r = 0; e < 24; e++) {
	[n, r] = [r, (2 * n + 3 * r) % 5], Dn.push(2 * (5 * r + n)), On.push((e + 1) * (e + 2) / 2 % 64);
	let i = An;
	for (let e = 0; e < 7; e++) t = (t << jn ^ (t >> Nn) * Fn) % Pn, t & Mn && (i ^= jn << (jn << /* @__PURE__ */ BigInt(e)) - jn);
	kn.push(i);
}
var [In, Ln] = /* @__PURE__ */ $t(kn, !0), Rn = (e, t, n) => n > 32 ? fn(e, t, n) : un(e, t, n), zn = (e, t, n) => n > 32 ? pn(e, t, n) : dn(e, t, n);
function Bn(e, t = 24) {
	let n = /* @__PURE__ */ new Uint32Array(10);
	for (let r = 24 - t; r < 24; r++) {
		for (let t = 0; t < 10; t++) n[t] = e[t] ^ e[t + 10] ^ e[t + 20] ^ e[t + 30] ^ e[t + 40];
		for (let t = 0; t < 10; t += 2) {
			let r = (t + 8) % 10, i = (t + 2) % 10, a = n[i], o = n[i + 1], s = Rn(a, o, 1) ^ n[r], c = zn(a, o, 1) ^ n[r + 1];
			for (let n = 0; n < 50; n += 10) e[t + n] ^= s, e[t + n + 1] ^= c;
		}
		let t = e[2], i = e[3];
		for (let n = 0; n < 24; n++) {
			let r = On[n], a = Rn(t, i, r), o = zn(t, i, r), s = Dn[n];
			t = e[s], i = e[s + 1], e[s] = a, e[s + 1] = o;
		}
		for (let t = 0; t < 50; t += 10) {
			for (let r = 0; r < 10; r++) n[r] = e[t + r];
			for (let r = 0; r < 10; r++) e[t + r] ^= ~n[(r + 2) % 10] & n[(r + 4) % 10];
		}
		e[0] ^= In[r], e[1] ^= Ln[r];
	}
	n.fill(0);
}
var Vn = class e extends jt {
	constructor(e, t, n, r = !1, i = 24) {
		if (super(), this.blockLen = e, this.suffix = t, this.outputLen = n, this.enableXOF = r, this.rounds = i, this.pos = 0, this.posOut = 0, this.finished = !1, this.destroyed = !1, gt(n), 0 >= this.blockLen || this.blockLen >= 200) throw Error("Sha3 supports only keccak-f1600 function");
		this.state = /* @__PURE__ */ new Uint8Array(200), this.state32 = Ct(this.state);
	}
	keccak() {
		Bn(this.state32, this.rounds), this.posOut = 0, this.pos = 0;
	}
	update(e) {
		yt(this);
		let { blockLen: t, state: n } = this;
		e = kt(e);
		let r = e.length;
		for (let i = 0; i < r;) {
			let a = Math.min(t - this.pos, r - i);
			for (let t = 0; t < a; t++) n[this.pos++] ^= e[i++];
			this.pos === t && this.keccak();
		}
		return this;
	}
	finish() {
		if (this.finished) return;
		this.finished = !0;
		let { state: e, suffix: t, pos: n, blockLen: r } = this;
		e[n] ^= t, t & 128 && n === r - 1 && this.keccak(), e[r - 1] ^= 128, this.keccak();
	}
	writeInto(e) {
		yt(this, !1), _t(e), this.finish();
		let t = this.state, { blockLen: n } = this;
		for (let r = 0, i = e.length; r < i;) {
			this.posOut >= n && this.keccak();
			let a = Math.min(n - this.posOut, i - r);
			e.set(t.subarray(this.posOut, this.posOut + a), r), this.posOut += a, r += a;
		}
		return e;
	}
	xofInto(e) {
		if (!this.enableXOF) throw Error("XOF is not possible for this instance");
		return this.writeInto(e);
	}
	xof(e) {
		return gt(e), this.xofInto(new Uint8Array(e));
	}
	digestInto(e) {
		if (bt(e, this), this.finished) throw Error("digest() was already called");
		return this.writeInto(e), this.destroy(), e;
	}
	digest() {
		return this.digestInto(new Uint8Array(this.outputLen));
	}
	destroy() {
		this.destroyed = !0, this.state.fill(0);
	}
	_cloneInto(t) {
		let { blockLen: n, suffix: r, outputLen: i, rounds: a, enableXOF: o } = this;
		return t ||= new e(n, r, i, o, a), t.state32.set(this.state32), t.pos = this.pos, t.posOut = this.posOut, t.finished = this.finished, t.rounds = a, t.suffix = r, t.outputLen = i, t.enableXOF = o, t.destroyed = this.destroyed, t;
	}
}, Hn = /* @__PURE__ */ ((e, t, n) => Pt(() => new Vn(t, e, n)))(1, 136, 32), Un = !1, Wn = function(e) {
	return Hn(e);
}, Gn = Wn;
function W(e) {
	let t = _(e, "data");
	return S(Gn(t));
}
W._ = Wn, W.lock = function() {
	Un = !0;
}, W.register = function(e) {
	if (Un) throw TypeError("keccak256 is locked");
	Gn = e;
}, Object.freeze(W);
//#endregion
//#region node_modules/ethers/lib.esm/crypto/pbkdf2.js
var Kn = !1, qn = function(e, t, n, r, i) {
	return Tn(e, t, n, r, i);
}, Jn = qn;
function Yn(e, t, n, r, i) {
	let a = _(e, "password"), o = _(t, "salt");
	return S(Jn(a, o, n, r, i));
}
Yn._ = qn, Yn.lock = function() {
	Kn = !0;
}, Yn.register = function(e) {
	if (Kn) throw Error("pbkdf2 is locked");
	Jn = e;
}, Object.freeze(Yn);
//#endregion
//#region node_modules/ethers/lib.esm/crypto/random.js
var Xn = !1, Zn = function(e) {
	return new Uint8Array(En(e));
}, Qn = Zn;
function $n(e) {
	return Qn(e);
}
$n._ = Zn, $n.lock = function() {
	Xn = !0;
}, $n.register = function(e) {
	if (Xn) throw Error("randomBytes is locked");
	Qn = e;
}, Object.freeze($n);
//#endregion
//#region node_modules/@noble/hashes/esm/scrypt.js
var G = (e, t) => e << t | e >>> 32 - t;
function er(e, t, n, r, i, a) {
	let o = e[t++] ^ n[r++], s = e[t++] ^ n[r++], c = e[t++] ^ n[r++], l = e[t++] ^ n[r++], u = e[t++] ^ n[r++], d = e[t++] ^ n[r++], f = e[t++] ^ n[r++], p = e[t++] ^ n[r++], m = e[t++] ^ n[r++], h = e[t++] ^ n[r++], g = e[t++] ^ n[r++], _ = e[t++] ^ n[r++], v = e[t++] ^ n[r++], y = e[t++] ^ n[r++], b = e[t++] ^ n[r++], x = e[t++] ^ n[r++], S = o, C = s, w = c, T = l, E = u, D = d, O = f, k = p, A = m, j = h, M = g, N = _, P = v, F = y, I = b, L = x;
	for (let e = 0; e < 8; e += 2) E ^= G(S + P | 0, 7), A ^= G(E + S | 0, 9), P ^= G(A + E | 0, 13), S ^= G(P + A | 0, 18), j ^= G(D + C | 0, 7), F ^= G(j + D | 0, 9), C ^= G(F + j | 0, 13), D ^= G(C + F | 0, 18), I ^= G(M + O | 0, 7), w ^= G(I + M | 0, 9), O ^= G(w + I | 0, 13), M ^= G(O + w | 0, 18), T ^= G(L + N | 0, 7), k ^= G(T + L | 0, 9), N ^= G(k + T | 0, 13), L ^= G(N + k | 0, 18), C ^= G(S + T | 0, 7), w ^= G(C + S | 0, 9), T ^= G(w + C | 0, 13), S ^= G(T + w | 0, 18), O ^= G(D + E | 0, 7), k ^= G(O + D | 0, 9), E ^= G(k + O | 0, 13), D ^= G(E + k | 0, 18), N ^= G(M + j | 0, 7), A ^= G(N + M | 0, 9), j ^= G(A + N | 0, 13), M ^= G(j + A | 0, 18), P ^= G(L + I | 0, 7), F ^= G(P + L | 0, 9), I ^= G(F + P | 0, 13), L ^= G(I + F | 0, 18);
	i[a++] = o + S | 0, i[a++] = s + C | 0, i[a++] = c + w | 0, i[a++] = l + T | 0, i[a++] = u + E | 0, i[a++] = d + D | 0, i[a++] = f + O | 0, i[a++] = p + k | 0, i[a++] = m + A | 0, i[a++] = h + j | 0, i[a++] = g + M | 0, i[a++] = _ + N | 0, i[a++] = v + P | 0, i[a++] = y + F | 0, i[a++] = b + I | 0, i[a++] = x + L | 0;
}
function tr(e, t, n, r, i) {
	let a = r + 0, o = r + 16 * i;
	for (let r = 0; r < 16; r++) n[o + r] = e[t + (2 * i - 1) * 16 + r];
	for (let r = 0; r < i; r++, a += 16, t += 16) er(n, o, e, t, n, a), r > 0 && (o += 16), er(n, a, e, t += 16, n, o);
}
function nr(e, t, n) {
	let { N: r, r: i, p: a, dkLen: o, asyncTick: s, maxmem: c, onProgress: l } = Nt({
		dkLen: 32,
		asyncTick: 10,
		maxmem: 1024 ** 3 + 1024
	}, n);
	if (gt(r), gt(i), gt(a), gt(o), gt(s), gt(c), l !== void 0 && typeof l != "function") throw Error("progressCb should be function");
	let u = 128 * i, d = u / 4;
	if (r <= 1 || r & r - 1 || r >= 2 ** (u / 8) || r > 2 ** 32) throw Error("Scrypt: N must be larger than 1, a power of 2, less than 2^(128 * r / 8) and less than 2^32");
	if (a < 0 || a > (2 ** 32 - 1) * 32 / u) throw Error("Scrypt: p must be a positive integer less than or equal to ((2^32 - 1) * 32) / (128 * r)");
	if (o < 0 || o > (2 ** 32 - 1) * 32) throw Error("Scrypt: dkLen should be positive integer less than or equal to (2^32 - 1) * 32");
	let f = u * (r + a);
	if (f > c) throw Error(`Scrypt: parameters too large, ${f} (128 * r * (N + p)) > ${c} (maxmem)`);
	let p = Bt(Yt, e, t, {
		c: 1,
		dkLen: u * a
	}), m = Ct(p), h = Ct(new Uint8Array(u * r)), g = Ct(new Uint8Array(u)), _ = () => {};
	if (l) {
		let e = 2 * r * a, t = Math.max(Math.floor(e / 1e4), 1), n = 0;
		_ = () => {
			n++, l && (!(n % t) || n === e) && l(n / e);
		};
	}
	return {
		N: r,
		r: i,
		p: a,
		dkLen: o,
		blockSize32: d,
		V: h,
		B32: m,
		B: p,
		tmp: g,
		blockMixCb: _,
		asyncTick: s
	};
}
function rr(e, t, n, r, i) {
	let a = Bt(Yt, e, n, {
		c: 1,
		dkLen: t
	});
	return n.fill(0), r.fill(0), i.fill(0), a;
}
function ir(e, t, n) {
	let { N: r, r: i, p: a, dkLen: o, blockSize32: s, V: c, B32: l, B: u, tmp: d, blockMixCb: f } = nr(e, t, n);
	for (let e = 0; e < a; e++) {
		let t = s * e;
		for (let e = 0; e < s; e++) c[e] = l[t + e];
		for (let e = 0, t = 0; e < r - 1; e++) tr(c, t, c, t += s, i), f();
		tr(c, (r - 1) * s, l, t, i), f();
		for (let e = 0; e < r; e++) {
			let e = l[t + s - 16] % r;
			for (let n = 0; n < s; n++) d[n] = l[t + n] ^ c[e * s + n];
			tr(d, 0, l, t, i), f();
		}
	}
	return rr(e, o, u, c, d);
}
async function ar(e, t, n) {
	let { N: r, r: i, p: a, dkLen: o, blockSize32: s, V: c, B32: l, B: u, tmp: d, blockMixCb: f, asyncTick: p } = nr(e, t, n);
	for (let e = 0; e < a; e++) {
		let t = s * e;
		for (let e = 0; e < s; e++) c[e] = l[t + e];
		let n = 0;
		await Dt(r - 1, p, () => {
			tr(c, n, c, n += s, i), f();
		}), tr(c, (r - 1) * s, l, t, i), f(), await Dt(r, p, () => {
			let e = l[t + s - 16] % r;
			for (let n = 0; n < s; n++) d[n] = l[t + n] ^ c[e * s + n];
			tr(d, 0, l, t, i), f();
		});
	}
	return rr(e, o, u, c, d);
}
//#endregion
//#region node_modules/ethers/lib.esm/crypto/scrypt.js
var or = !1, sr = !1, cr = async function(e, t, n, r, i, a, o) {
	return await ar(e, t, {
		N: n,
		r,
		p: i,
		dkLen: a,
		onProgress: o
	});
}, lr = function(e, t, n, r, i, a) {
	return ir(e, t, {
		N: n,
		r,
		p: i,
		dkLen: a
	});
}, ur = cr, dr = lr;
async function fr(e, t, n, r, i, a, o) {
	let s = _(e, "passwd"), c = _(t, "salt");
	return S(await ur(s, c, n, r, i, a, o));
}
fr._ = cr, fr.lock = function() {
	sr = !0;
}, fr.register = function(e) {
	if (sr) throw Error("scrypt is locked");
	ur = e;
}, Object.freeze(fr);
function pr(e, t, n, r, i, a) {
	let o = _(e, "passwd"), s = _(t, "salt");
	return S(dr(o, s, n, r, i, a));
}
pr._ = lr, pr.lock = function() {
	or = !0;
}, pr.register = function(e) {
	if (or) throw Error("scryptSync is locked");
	dr = e;
}, Object.freeze(pr);
//#endregion
//#region node_modules/ethers/lib.esm/crypto/sha2.js
var mr = function(e) {
	return wn("sha256").update(e).digest();
}, hr = mr, gr = !1;
function _r(e) {
	let t = _(e, "data");
	return S(hr(t));
}
_r._ = mr, _r.lock = function() {
	gr = !0;
}, _r.register = function(e) {
	if (gr) throw Error("sha256 is locked");
	hr = e;
}, Object.freeze(_r), Object.freeze(_r);
//#endregion
//#region node_modules/@noble/curves/esm/abstract/utils.js
var vr = /* @__PURE__ */ t({
	bitGet: () => Rr,
	bitLen: () => Lr,
	bitMask: () => Br,
	bitSet: () => zr,
	bytesToHex: () => wr,
	bytesToNumberBE: () => Or,
	bytesToNumberLE: () => kr,
	concatBytes: () => Pr,
	createHmacDrbg: () => Ur,
	ensureBytes: () => Nr,
	equalBytes: () => Fr,
	hexToBytes: () => Dr,
	hexToNumber: () => Er,
	numberToBytesBE: () => Ar,
	numberToBytesLE: () => jr,
	numberToHexUnpadded: () => Tr,
	numberToVarBytesBE: () => Mr,
	utf8ToBytes: () => Ir,
	validateObject: () => Gr
}), yr = BigInt(0), br = BigInt(1), xr = BigInt(2), Sr = (e) => e instanceof Uint8Array, Cr = /* @__PURE__ */ Array.from({ length: 256 }, (e, t) => t.toString(16).padStart(2, "0"));
function wr(e) {
	if (!Sr(e)) throw Error("Uint8Array expected");
	let t = "";
	for (let n = 0; n < e.length; n++) t += Cr[e[n]];
	return t;
}
function Tr(e) {
	let t = e.toString(16);
	return t.length & 1 ? `0${t}` : t;
}
function Er(e) {
	if (typeof e != "string") throw Error("hex string expected, got " + typeof e);
	return BigInt(e === "" ? "0" : `0x${e}`);
}
function Dr(e) {
	if (typeof e != "string") throw Error("hex string expected, got " + typeof e);
	let t = e.length;
	if (t % 2) throw Error("padded hex string expected, got unpadded hex of length " + t);
	let n = new Uint8Array(t / 2);
	for (let t = 0; t < n.length; t++) {
		let r = t * 2, i = e.slice(r, r + 2), a = Number.parseInt(i, 16);
		if (Number.isNaN(a) || a < 0) throw Error("Invalid byte sequence");
		n[t] = a;
	}
	return n;
}
function Or(e) {
	return Er(wr(e));
}
function kr(e) {
	if (!Sr(e)) throw Error("Uint8Array expected");
	return Er(wr(Uint8Array.from(e).reverse()));
}
function Ar(e, t) {
	return Dr(e.toString(16).padStart(t * 2, "0"));
}
function jr(e, t) {
	return Ar(e, t).reverse();
}
function Mr(e) {
	return Dr(Tr(e));
}
function Nr(e, t, n) {
	let r;
	if (typeof t == "string") try {
		r = Dr(t);
	} catch (n) {
		throw Error(`${e} must be valid hex string, got "${t}". Cause: ${n}`);
	}
	else if (Sr(t)) r = Uint8Array.from(t);
	else throw Error(`${e} must be hex string or Uint8Array`);
	let i = r.length;
	if (typeof n == "number" && i !== n) throw Error(`${e} expected ${n} bytes, got ${i}`);
	return r;
}
function Pr(...e) {
	let t = new Uint8Array(e.reduce((e, t) => e + t.length, 0)), n = 0;
	return e.forEach((e) => {
		if (!Sr(e)) throw Error("Uint8Array expected");
		t.set(e, n), n += e.length;
	}), t;
}
function Fr(e, t) {
	if (e.length !== t.length) return !1;
	for (let n = 0; n < e.length; n++) if (e[n] !== t[n]) return !1;
	return !0;
}
function Ir(e) {
	if (typeof e != "string") throw Error(`utf8ToBytes expected string, got ${typeof e}`);
	return new Uint8Array(new TextEncoder().encode(e));
}
function Lr(e) {
	let t = 0;
	for (; e > yr; e >>= br, t += 1);
	return t;
}
function Rr(e, t) {
	return e >> BigInt(t) & br;
}
var zr = (e, t, n) => e | (n ? br : yr) << BigInt(t), Br = (e) => (xr << BigInt(e - 1)) - br, Vr = (e) => new Uint8Array(e), Hr = (e) => Uint8Array.from(e);
function Ur(e, t, n) {
	if (typeof e != "number" || e < 2) throw Error("hashLen must be a number");
	if (typeof t != "number" || t < 2) throw Error("qByteLen must be a number");
	if (typeof n != "function") throw Error("hmacFn must be a function");
	let r = Vr(e), i = Vr(e), a = 0, o = () => {
		r.fill(1), i.fill(0), a = 0;
	}, s = (...e) => n(i, r, ...e), c = (e = Vr()) => {
		i = s(Hr([0]), e), r = s(), e.length !== 0 && (i = s(Hr([1]), e), r = s());
	}, l = () => {
		if (a++ >= 1e3) throw Error("drbg: tried 1000 values");
		let e = 0, n = [];
		for (; e < t;) {
			r = s();
			let t = r.slice();
			n.push(t), e += r.length;
		}
		return Pr(...n);
	};
	return (e, t) => {
		o(), c(e);
		let n;
		for (; !(n = t(l()));) c();
		return o(), n;
	};
}
var Wr = {
	bigint: (e) => typeof e == "bigint",
	function: (e) => typeof e == "function",
	boolean: (e) => typeof e == "boolean",
	string: (e) => typeof e == "string",
	stringOrUint8Array: (e) => typeof e == "string" || e instanceof Uint8Array,
	isSafeInteger: (e) => Number.isSafeInteger(e),
	array: (e) => Array.isArray(e),
	field: (e, t) => t.Fp.isValid(e),
	hash: (e) => typeof e == "function" && Number.isSafeInteger(e.outputLen)
};
function Gr(e, t, n = {}) {
	let r = (t, n, r) => {
		let i = Wr[n];
		if (typeof i != "function") throw Error(`Invalid validator "${n}", expected function`);
		let a = e[t];
		if (!(r && a === void 0) && !i(a, e)) throw Error(`Invalid param ${String(t)}=${a} (${typeof a}), expected ${n}`);
	};
	for (let [e, n] of Object.entries(t)) r(e, n, !1);
	for (let [e, t] of Object.entries(n)) r(e, t, !0);
	return e;
}
//#endregion
//#region node_modules/@noble/curves/esm/abstract/modular.js
var Kr = BigInt(0), qr = BigInt(1), Jr = BigInt(2), Yr = BigInt(3), Xr = BigInt(4), Zr = BigInt(5), Qr = BigInt(8), $r = BigInt(16);
function ei(e, t) {
	let n = e % t;
	return n >= Kr ? n : t + n;
}
function ti(e, t, n) {
	if (n <= Kr || t < Kr) throw Error("Expected power/modulo > 0");
	if (n === qr) return Kr;
	let r = qr;
	for (; t > Kr;) t & qr && (r = r * e % n), e = e * e % n, t >>= qr;
	return r;
}
function ni(e, t, n) {
	let r = e;
	for (; t-- > Kr;) r *= r, r %= n;
	return r;
}
function ri(e, t) {
	if (e === Kr || t <= Kr) throw Error(`invert: expected positive integers, got n=${e} mod=${t}`);
	let n = ei(e, t), r = t, i = Kr, a = qr, o = qr, s = Kr;
	for (; n !== Kr;) {
		let e = r / n, t = r % n, c = i - o * e, l = a - s * e;
		r = n, n = t, i = o, a = s, o = c, s = l;
	}
	if (r !== qr) throw Error("invert: does not exist");
	return ei(i, t);
}
function ii(e) {
	let t = (e - qr) / Jr, n, r, i;
	for (n = e - qr, r = 0; n % Jr === Kr; n /= Jr, r++);
	for (i = Jr; i < e && ti(i, t, e) !== e - qr; i++);
	if (r === 1) {
		let t = (e + qr) / Xr;
		return function(e, n) {
			let r = e.pow(n, t);
			if (!e.eql(e.sqr(r), n)) throw Error("Cannot find square root");
			return r;
		};
	}
	let a = (n + qr) / Jr;
	return function(e, o) {
		if (e.pow(o, t) === e.neg(e.ONE)) throw Error("Cannot find square root");
		let s = r, c = e.pow(e.mul(e.ONE, i), n), l = e.pow(o, a), u = e.pow(o, n);
		for (; !e.eql(u, e.ONE);) {
			if (e.eql(u, e.ZERO)) return e.ZERO;
			let t = 1;
			for (let n = e.sqr(u); t < s && !e.eql(n, e.ONE); t++) n = e.sqr(n);
			let n = e.pow(c, qr << BigInt(s - t - 1));
			c = e.sqr(n), l = e.mul(l, n), u = e.mul(u, c), s = t;
		}
		return l;
	};
}
function ai(e) {
	if (e % Xr === Yr) {
		let t = (e + qr) / Xr;
		return function(e, n) {
			let r = e.pow(n, t);
			if (!e.eql(e.sqr(r), n)) throw Error("Cannot find square root");
			return r;
		};
	}
	if (e % Qr === Zr) {
		let t = (e - Zr) / Qr;
		return function(e, n) {
			let r = e.mul(n, Jr), i = e.pow(r, t), a = e.mul(n, i), o = e.mul(e.mul(a, Jr), i), s = e.mul(a, e.sub(o, e.ONE));
			if (!e.eql(e.sqr(s), n)) throw Error("Cannot find square root");
			return s;
		};
	}
	return e % $r, ii(e);
}
var oi = (e, t) => (ei(e, t) & qr) === qr, si = [
	"create",
	"isValid",
	"is0",
	"neg",
	"inv",
	"sqrt",
	"sqr",
	"eql",
	"add",
	"sub",
	"mul",
	"pow",
	"div",
	"addN",
	"subN",
	"mulN",
	"sqrN"
];
function ci(e) {
	return Gr(e, si.reduce((e, t) => (e[t] = "function", e), {
		ORDER: "bigint",
		MASK: "bigint",
		BYTES: "isSafeInteger",
		BITS: "isSafeInteger"
	}));
}
function li(e, t, n) {
	if (n < Kr) throw Error("Expected power > 0");
	if (n === Kr) return e.ONE;
	if (n === qr) return t;
	let r = e.ONE, i = t;
	for (; n > Kr;) n & qr && (r = e.mul(r, i)), i = e.sqr(i), n >>= qr;
	return r;
}
function ui(e, t) {
	let n = Array(t.length), r = t.reduce((t, r, i) => e.is0(r) ? t : (n[i] = t, e.mul(t, r)), e.ONE), i = e.inv(r);
	return t.reduceRight((t, r, i) => e.is0(r) ? t : (n[i] = e.mul(t, n[i]), e.mul(t, r)), i), n;
}
function di(e, t) {
	let n = t === void 0 ? e.toString(2).length : t;
	return {
		nBitLength: n,
		nByteLength: Math.ceil(n / 8)
	};
}
function fi(e, t, n = !1, r = {}) {
	if (e <= Kr) throw Error(`Expected Field ORDER > 0, got ${e}`);
	let { nBitLength: i, nByteLength: a } = di(e, t);
	if (a > 2048) throw Error("Field lengths over 2048 bytes are not supported");
	let o = ai(e), s = Object.freeze({
		ORDER: e,
		BITS: i,
		BYTES: a,
		MASK: Br(i),
		ZERO: Kr,
		ONE: qr,
		create: (t) => ei(t, e),
		isValid: (t) => {
			if (typeof t != "bigint") throw Error(`Invalid field element: expected bigint, got ${typeof t}`);
			return Kr <= t && t < e;
		},
		is0: (e) => e === Kr,
		isOdd: (e) => (e & qr) === qr,
		neg: (t) => ei(-t, e),
		eql: (e, t) => e === t,
		sqr: (t) => ei(t * t, e),
		add: (t, n) => ei(t + n, e),
		sub: (t, n) => ei(t - n, e),
		mul: (t, n) => ei(t * n, e),
		pow: (e, t) => li(s, e, t),
		div: (t, n) => ei(t * ri(n, e), e),
		sqrN: (e) => e * e,
		addN: (e, t) => e + t,
		subN: (e, t) => e - t,
		mulN: (e, t) => e * t,
		inv: (t) => ri(t, e),
		sqrt: r.sqrt || ((e) => o(s, e)),
		invertBatch: (e) => ui(s, e),
		cmov: (e, t, n) => n ? t : e,
		toBytes: (e) => n ? jr(e, a) : Ar(e, a),
		fromBytes: (e) => {
			if (e.length !== a) throw Error(`Fp.fromBytes: expected ${a}, got ${e.length}`);
			return n ? kr(e) : Or(e);
		}
	});
	return Object.freeze(s);
}
function pi(e, t) {
	if (!e.isOdd) throw Error("Field doesn't have isOdd");
	let n = e.sqrt(t);
	return e.isOdd(n) ? e.neg(n) : n;
}
function mi(e) {
	if (typeof e != "bigint") throw Error("field order must be bigint");
	let t = e.toString(2).length;
	return Math.ceil(t / 8);
}
function hi(e) {
	let t = mi(e);
	return t + Math.ceil(t / 2);
}
function gi(e, t, n = !1) {
	let r = e.length, i = mi(t), a = hi(t);
	if (r < 16 || r < a || r > 1024) throw Error(`expected ${a}-1024 bytes of input, got ${r}`);
	let o = ei(n ? Or(e) : kr(e), t - qr) + qr;
	return n ? jr(o, i) : Ar(o, i);
}
//#endregion
//#region node_modules/@noble/curves/esm/abstract/curve.js
var _i = BigInt(0), vi = BigInt(1);
function yi(e, t) {
	let n = (e, t) => {
		let n = t.negate();
		return e ? n : t;
	}, r = (e) => ({
		windows: Math.ceil(t / e) + 1,
		windowSize: 2 ** (e - 1)
	});
	return {
		constTimeNegate: n,
		unsafeLadder(t, n) {
			let r = e.ZERO, i = t;
			for (; n > _i;) n & vi && (r = r.add(i)), i = i.double(), n >>= vi;
			return r;
		},
		precomputeWindow(e, t) {
			let { windows: n, windowSize: i } = r(t), a = [], o = e, s = o;
			for (let e = 0; e < n; e++) {
				s = o, a.push(s);
				for (let e = 1; e < i; e++) s = s.add(o), a.push(s);
				o = s.double();
			}
			return a;
		},
		wNAF(t, i, a) {
			let { windows: o, windowSize: s } = r(t), c = e.ZERO, l = e.BASE, u = BigInt(2 ** t - 1), d = 2 ** t, f = BigInt(t);
			for (let e = 0; e < o; e++) {
				let t = e * s, r = Number(a & u);
				a >>= f, r > s && (r -= d, a += vi);
				let o = t, p = t + Math.abs(r) - 1, m = e % 2 != 0, h = r < 0;
				r === 0 ? l = l.add(n(m, i[o])) : c = c.add(n(h, i[p]));
			}
			return {
				p: c,
				f: l
			};
		},
		wNAFCached(e, t, n, r) {
			let i = e._WINDOW_SIZE || 1, a = t.get(e);
			return a || (a = this.precomputeWindow(e, i), i !== 1 && t.set(e, r(a))), this.wNAF(i, a, n);
		}
	};
}
function bi(e) {
	return ci(e.Fp), Gr(e, {
		n: "bigint",
		h: "bigint",
		Gx: "field",
		Gy: "field"
	}, {
		nBitLength: "isSafeInteger",
		nByteLength: "isSafeInteger"
	}), Object.freeze({
		...di(e.n, e.nBitLength),
		...e,
		p: e.Fp.ORDER
	});
}
//#endregion
//#region node_modules/@noble/curves/esm/abstract/weierstrass.js
function xi(e) {
	let t = bi(e);
	Gr(t, {
		a: "field",
		b: "field"
	}, {
		allowedPrivateKeyLengths: "array",
		wrapPrivateKey: "boolean",
		isTorsionFree: "function",
		clearCofactor: "function",
		allowInfinityPoint: "boolean",
		fromBytes: "function",
		toBytes: "function"
	});
	let { endo: n, Fp: r, a: i } = t;
	if (n) {
		if (!r.eql(i, r.ZERO)) throw Error("Endomorphism can only be defined for Koblitz curves that have a=0");
		if (typeof n != "object" || typeof n.beta != "bigint" || typeof n.splitScalar != "function") throw Error("Expected endomorphism with beta: bigint and splitScalar: function");
	}
	return Object.freeze({ ...t });
}
var { bytesToNumberBE: Si, hexToBytes: Ci } = vr, wi = {
	Err: class extends Error {
		constructor(e = "") {
			super(e);
		}
	},
	_parseInt(e) {
		let { Err: t } = wi;
		if (e.length < 2 || e[0] !== 2) throw new t("Invalid signature integer tag");
		let n = e[1], r = e.subarray(2, n + 2);
		if (!n || r.length !== n) throw new t("Invalid signature integer: wrong length");
		if (r[0] & 128) throw new t("Invalid signature integer: negative");
		if (r[0] === 0 && !(r[1] & 128)) throw new t("Invalid signature integer: unnecessary leading zero");
		return {
			d: Si(r),
			l: e.subarray(n + 2)
		};
	},
	toSig(e) {
		let { Err: t } = wi, n = typeof e == "string" ? Ci(e) : e;
		if (!(n instanceof Uint8Array)) throw Error("ui8a expected");
		let r = n.length;
		if (r < 2 || n[0] != 48) throw new t("Invalid signature tag");
		if (n[1] !== r - 2) throw new t("Invalid signature: incorrect length");
		let { d: i, l: a } = wi._parseInt(n.subarray(2)), { d: o, l: s } = wi._parseInt(a);
		if (s.length) throw new t("Invalid signature: left bytes after parsing");
		return {
			r: i,
			s: o
		};
	},
	hexFromSig(e) {
		let t = (e) => Number.parseInt(e[0], 16) & 8 ? "00" + e : e, n = (e) => {
			let t = e.toString(16);
			return t.length & 1 ? `0${t}` : t;
		}, r = t(n(e.s)), i = t(n(e.r)), a = r.length / 2, o = i.length / 2, s = n(a), c = n(o);
		return `30${n(o + a + 4)}02${c}${i}02${s}${r}`;
	}
}, Ti = BigInt(0), Ei = BigInt(1), Di = BigInt(3);
function Oi(e) {
	let t = xi(e), { Fp: n } = t, r = t.toBytes || ((e, t, r) => {
		let i = t.toAffine();
		return Pr(Uint8Array.from([4]), n.toBytes(i.x), n.toBytes(i.y));
	}), i = t.fromBytes || ((e) => {
		let t = e.subarray(1);
		return {
			x: n.fromBytes(t.subarray(0, n.BYTES)),
			y: n.fromBytes(t.subarray(n.BYTES, 2 * n.BYTES))
		};
	});
	function a(e) {
		let { a: r, b: i } = t, a = n.sqr(e), o = n.mul(a, e);
		return n.add(n.add(o, n.mul(e, r)), i);
	}
	if (!n.eql(n.sqr(t.Gy), a(t.Gx))) throw Error("bad generator point: equation left != right");
	function o(e) {
		return typeof e == "bigint" && Ti < e && e < t.n;
	}
	function s(e) {
		if (!o(e)) throw Error("Expected valid bigint: 0 < bigint < curve.n");
	}
	function c(e) {
		let { allowedPrivateKeyLengths: n, nByteLength: r, wrapPrivateKey: i, n: a } = t;
		if (n && typeof e != "bigint") {
			if (e instanceof Uint8Array && (e = wr(e)), typeof e != "string" || !n.includes(e.length)) throw Error("Invalid key");
			e = e.padStart(r * 2, "0");
		}
		let o;
		try {
			o = typeof e == "bigint" ? e : Or(Nr("private key", e, r));
		} catch {
			throw Error(`private key must be ${r} bytes, hex or bigint, not ${typeof e}`);
		}
		return i && (o = ei(o, a)), s(o), o;
	}
	let l = /* @__PURE__ */ new Map();
	function u(e) {
		if (!(e instanceof d)) throw Error("ProjectivePoint expected");
	}
	class d {
		constructor(e, t, r) {
			if (this.px = e, this.py = t, this.pz = r, e == null || !n.isValid(e)) throw Error("x required");
			if (t == null || !n.isValid(t)) throw Error("y required");
			if (r == null || !n.isValid(r)) throw Error("z required");
		}
		static fromAffine(e) {
			let { x: t, y: r } = e || {};
			if (!e || !n.isValid(t) || !n.isValid(r)) throw Error("invalid affine point");
			if (e instanceof d) throw Error("projective point not allowed");
			let i = (e) => n.eql(e, n.ZERO);
			return i(t) && i(r) ? d.ZERO : new d(t, r, n.ONE);
		}
		get x() {
			return this.toAffine().x;
		}
		get y() {
			return this.toAffine().y;
		}
		static normalizeZ(e) {
			let t = n.invertBatch(e.map((e) => e.pz));
			return e.map((e, n) => e.toAffine(t[n])).map(d.fromAffine);
		}
		static fromHex(e) {
			let t = d.fromAffine(i(Nr("pointHex", e)));
			return t.assertValidity(), t;
		}
		static fromPrivateKey(e) {
			return d.BASE.multiply(c(e));
		}
		_setWindowSize(e) {
			this._WINDOW_SIZE = e, l.delete(this);
		}
		assertValidity() {
			if (this.is0()) {
				if (t.allowInfinityPoint && !n.is0(this.py)) return;
				throw Error("bad point: ZERO");
			}
			let { x: e, y: r } = this.toAffine();
			if (!n.isValid(e) || !n.isValid(r)) throw Error("bad point: x or y not FE");
			let i = n.sqr(r), o = a(e);
			if (!n.eql(i, o)) throw Error("bad point: equation left != right");
			if (!this.isTorsionFree()) throw Error("bad point: not in prime-order subgroup");
		}
		hasEvenY() {
			let { y: e } = this.toAffine();
			if (n.isOdd) return !n.isOdd(e);
			throw Error("Field doesn't support isOdd");
		}
		equals(e) {
			u(e);
			let { px: t, py: r, pz: i } = this, { px: a, py: o, pz: s } = e, c = n.eql(n.mul(t, s), n.mul(a, i)), l = n.eql(n.mul(r, s), n.mul(o, i));
			return c && l;
		}
		negate() {
			return new d(this.px, n.neg(this.py), this.pz);
		}
		double() {
			let { a: e, b: r } = t, i = n.mul(r, Di), { px: a, py: o, pz: s } = this, c = n.ZERO, l = n.ZERO, u = n.ZERO, f = n.mul(a, a), p = n.mul(o, o), m = n.mul(s, s), h = n.mul(a, o);
			return h = n.add(h, h), u = n.mul(a, s), u = n.add(u, u), c = n.mul(e, u), l = n.mul(i, m), l = n.add(c, l), c = n.sub(p, l), l = n.add(p, l), l = n.mul(c, l), c = n.mul(h, c), u = n.mul(i, u), m = n.mul(e, m), h = n.sub(f, m), h = n.mul(e, h), h = n.add(h, u), u = n.add(f, f), f = n.add(u, f), f = n.add(f, m), f = n.mul(f, h), l = n.add(l, f), m = n.mul(o, s), m = n.add(m, m), f = n.mul(m, h), c = n.sub(c, f), u = n.mul(m, p), u = n.add(u, u), u = n.add(u, u), new d(c, l, u);
		}
		add(e) {
			u(e);
			let { px: r, py: i, pz: a } = this, { px: o, py: s, pz: c } = e, l = n.ZERO, f = n.ZERO, p = n.ZERO, m = t.a, h = n.mul(t.b, Di), g = n.mul(r, o), _ = n.mul(i, s), v = n.mul(a, c), y = n.add(r, i), b = n.add(o, s);
			y = n.mul(y, b), b = n.add(g, _), y = n.sub(y, b), b = n.add(r, a);
			let x = n.add(o, c);
			return b = n.mul(b, x), x = n.add(g, v), b = n.sub(b, x), x = n.add(i, a), l = n.add(s, c), x = n.mul(x, l), l = n.add(_, v), x = n.sub(x, l), p = n.mul(m, b), l = n.mul(h, v), p = n.add(l, p), l = n.sub(_, p), p = n.add(_, p), f = n.mul(l, p), _ = n.add(g, g), _ = n.add(_, g), v = n.mul(m, v), b = n.mul(h, b), _ = n.add(_, v), v = n.sub(g, v), v = n.mul(m, v), b = n.add(b, v), g = n.mul(_, b), f = n.add(f, g), g = n.mul(x, b), l = n.mul(y, l), l = n.sub(l, g), g = n.mul(y, _), p = n.mul(x, p), p = n.add(p, g), new d(l, f, p);
		}
		subtract(e) {
			return this.add(e.negate());
		}
		is0() {
			return this.equals(d.ZERO);
		}
		wNAF(e) {
			return p.wNAFCached(this, l, e, (e) => {
				let t = n.invertBatch(e.map((e) => e.pz));
				return e.map((e, n) => e.toAffine(t[n])).map(d.fromAffine);
			});
		}
		multiplyUnsafe(e) {
			let r = d.ZERO;
			if (e === Ti) return r;
			if (s(e), e === Ei) return this;
			let { endo: i } = t;
			if (!i) return p.unsafeLadder(this, e);
			let { k1neg: a, k1: o, k2neg: c, k2: l } = i.splitScalar(e), u = r, f = r, m = this;
			for (; o > Ti || l > Ti;) o & Ei && (u = u.add(m)), l & Ei && (f = f.add(m)), m = m.double(), o >>= Ei, l >>= Ei;
			return a && (u = u.negate()), c && (f = f.negate()), f = new d(n.mul(f.px, i.beta), f.py, f.pz), u.add(f);
		}
		multiply(e) {
			s(e);
			let r = e, i, a, { endo: o } = t;
			if (o) {
				let { k1neg: e, k1: t, k2neg: s, k2: c } = o.splitScalar(r), { p: l, f: u } = this.wNAF(t), { p: f, f: m } = this.wNAF(c);
				l = p.constTimeNegate(e, l), f = p.constTimeNegate(s, f), f = new d(n.mul(f.px, o.beta), f.py, f.pz), i = l.add(f), a = u.add(m);
			} else {
				let { p: e, f: t } = this.wNAF(r);
				i = e, a = t;
			}
			return d.normalizeZ([i, a])[0];
		}
		multiplyAndAddUnsafe(e, t, n) {
			let r = d.BASE, i = (e, t) => t === Ti || t === Ei || !e.equals(r) ? e.multiplyUnsafe(t) : e.multiply(t), a = i(this, t).add(i(e, n));
			return a.is0() ? void 0 : a;
		}
		toAffine(e) {
			let { px: t, py: r, pz: i } = this, a = this.is0();
			e ??= a ? n.ONE : n.inv(i);
			let o = n.mul(t, e), s = n.mul(r, e), c = n.mul(i, e);
			if (a) return {
				x: n.ZERO,
				y: n.ZERO
			};
			if (!n.eql(c, n.ONE)) throw Error("invZ was invalid");
			return {
				x: o,
				y: s
			};
		}
		isTorsionFree() {
			let { h: e, isTorsionFree: n } = t;
			if (e === Ei) return !0;
			if (n) return n(d, this);
			throw Error("isTorsionFree() has not been declared for the elliptic curve");
		}
		clearCofactor() {
			let { h: e, clearCofactor: n } = t;
			return e === Ei ? this : n ? n(d, this) : this.multiplyUnsafe(t.h);
		}
		toRawBytes(e = !0) {
			return this.assertValidity(), r(d, this, e);
		}
		toHex(e = !0) {
			return wr(this.toRawBytes(e));
		}
	}
	d.BASE = new d(t.Gx, t.Gy, n.ONE), d.ZERO = new d(n.ZERO, n.ONE, n.ZERO);
	let f = t.nBitLength, p = yi(d, t.endo ? Math.ceil(f / 2) : f);
	return {
		CURVE: t,
		ProjectivePoint: d,
		normPrivateKeyToScalar: c,
		weierstrassEquation: a,
		isWithinCurveOrder: o
	};
}
function ki(e) {
	let t = bi(e);
	return Gr(t, {
		hash: "hash",
		hmac: "function",
		randomBytes: "function"
	}, {
		bits2int: "function",
		bits2int_modN: "function",
		lowS: "boolean"
	}), Object.freeze({
		lowS: !0,
		...t
	});
}
function Ai(e) {
	let t = ki(e), { Fp: n, n: r } = t, i = n.BYTES + 1, a = 2 * n.BYTES + 1;
	function o(e) {
		return Ti < e && e < n.ORDER;
	}
	function s(e) {
		return ei(e, r);
	}
	function c(e) {
		return ri(e, r);
	}
	let { ProjectivePoint: l, normPrivateKeyToScalar: u, weierstrassEquation: d, isWithinCurveOrder: f } = Oi({
		...t,
		toBytes(e, t, r) {
			let i = t.toAffine(), a = n.toBytes(i.x), o = Pr;
			return r ? o(Uint8Array.from([t.hasEvenY() ? 2 : 3]), a) : o(Uint8Array.from([4]), a, n.toBytes(i.y));
		},
		fromBytes(e) {
			let t = e.length, r = e[0], s = e.subarray(1);
			if (t === i && (r === 2 || r === 3)) {
				let e = Or(s);
				if (!o(e)) throw Error("Point is not on curve");
				let t = d(e), i = n.sqrt(t), a = (i & Ei) === Ei;
				return (r & 1) == 1 !== a && (i = n.neg(i)), {
					x: e,
					y: i
				};
			}
			if (t === a && r === 4) return {
				x: n.fromBytes(s.subarray(0, n.BYTES)),
				y: n.fromBytes(s.subarray(n.BYTES, 2 * n.BYTES))
			};
			throw Error(`Point of length ${t} was invalid. Expected ${i} compressed bytes or ${a} uncompressed bytes`);
		}
	}), p = (e) => wr(Ar(e, t.nByteLength));
	function m(e) {
		return e > r >> Ei;
	}
	function h(e) {
		return m(e) ? s(-e) : e;
	}
	let g = (e, t, n) => Or(e.slice(t, n));
	class _ {
		constructor(e, t, n) {
			this.r = e, this.s = t, this.recovery = n, this.assertValidity();
		}
		static fromCompact(e) {
			let n = t.nByteLength;
			return e = Nr("compactSignature", e, n * 2), new _(g(e, 0, n), g(e, n, 2 * n));
		}
		static fromDER(e) {
			let { r: t, s: n } = wi.toSig(Nr("DER", e));
			return new _(t, n);
		}
		assertValidity() {
			if (!f(this.r)) throw Error("r must be 0 < r < CURVE.n");
			if (!f(this.s)) throw Error("s must be 0 < s < CURVE.n");
		}
		addRecoveryBit(e) {
			return new _(this.r, this.s, e);
		}
		recoverPublicKey(e) {
			let { r, s: i, recovery: a } = this, o = C(Nr("msgHash", e));
			if (a == null || ![
				0,
				1,
				2,
				3
			].includes(a)) throw Error("recovery id invalid");
			let u = a === 2 || a === 3 ? r + t.n : r;
			if (u >= n.ORDER) throw Error("recovery id 2 or 3 invalid");
			let d = a & 1 ? "03" : "02", f = l.fromHex(d + p(u)), m = c(u), h = s(-o * m), g = s(i * m), _ = l.BASE.multiplyAndAddUnsafe(f, h, g);
			if (!_) throw Error("point at infinify");
			return _.assertValidity(), _;
		}
		hasHighS() {
			return m(this.s);
		}
		normalizeS() {
			return this.hasHighS() ? new _(this.r, s(-this.s), this.recovery) : this;
		}
		toDERRawBytes() {
			return Dr(this.toDERHex());
		}
		toDERHex() {
			return wi.hexFromSig({
				r: this.r,
				s: this.s
			});
		}
		toCompactRawBytes() {
			return Dr(this.toCompactHex());
		}
		toCompactHex() {
			return p(this.r) + p(this.s);
		}
	}
	let v = {
		isValidPrivateKey(e) {
			try {
				return u(e), !0;
			} catch {
				return !1;
			}
		},
		normPrivateKeyToScalar: u,
		randomPrivateKey: () => {
			let e = hi(t.n);
			return gi(t.randomBytes(e), t.n);
		},
		precompute(e = 8, t = l.BASE) {
			return t._setWindowSize(e), t.multiply(BigInt(3)), t;
		}
	};
	function y(e, t = !0) {
		return l.fromPrivateKey(e).toRawBytes(t);
	}
	function b(e) {
		let t = e instanceof Uint8Array, n = typeof e == "string", r = (t || n) && e.length;
		return t ? r === i || r === a : n ? r === 2 * i || r === 2 * a : e instanceof l;
	}
	function x(e, t, n = !0) {
		if (b(e)) throw Error("first arg must be private key");
		if (!b(t)) throw Error("second arg must be public key");
		return l.fromHex(t).multiply(u(e)).toRawBytes(n);
	}
	let S = t.bits2int || function(e) {
		let n = Or(e), r = e.length * 8 - t.nBitLength;
		return r > 0 ? n >> BigInt(r) : n;
	}, C = t.bits2int_modN || function(e) {
		return s(S(e));
	}, w = Br(t.nBitLength);
	function T(e) {
		if (typeof e != "bigint") throw Error("bigint expected");
		if (!(Ti <= e && e < w)) throw Error(`bigint expected < 2^${t.nBitLength}`);
		return Ar(e, t.nByteLength);
	}
	function E(e, r, i = D) {
		if (["recovered", "canonical"].some((e) => e in i)) throw Error("sign() legacy options not supported");
		let { hash: a, randomBytes: o } = t, { lowS: d, prehash: p, extraEntropy: g } = i;
		d ??= !0, e = Nr("msgHash", e), p && (e = Nr("prehashed msgHash", a(e)));
		let v = C(e), y = u(r), b = [T(y), T(v)];
		if (g != null) {
			let e = g === !0 ? o(n.BYTES) : g;
			b.push(Nr("extraEntropy", e));
		}
		let x = Pr(...b), w = v;
		function E(e) {
			let t = S(e);
			if (!f(t)) return;
			let n = c(t), r = l.BASE.multiply(t).toAffine(), i = s(r.x);
			if (i === Ti) return;
			let a = s(n * s(w + i * y));
			if (a === Ti) return;
			let o = (r.x === i ? 0 : 2) | Number(r.y & Ei), u = a;
			return d && m(a) && (u = h(a), o ^= 1), new _(i, u, o);
		}
		return {
			seed: x,
			k2sig: E
		};
	}
	let D = {
		lowS: t.lowS,
		prehash: !1
	}, O = {
		lowS: t.lowS,
		prehash: !1
	};
	function k(e, n, r = D) {
		let { seed: i, k2sig: a } = E(e, n, r), o = t;
		return Ur(o.hash.outputLen, o.nByteLength, o.hmac)(i, a);
	}
	l.BASE._setWindowSize(8);
	function A(e, n, r, i = O) {
		let a = e;
		if (n = Nr("msgHash", n), r = Nr("publicKey", r), "strict" in i) throw Error("options.strict was renamed to lowS");
		let { lowS: o, prehash: u } = i, d, f;
		try {
			if (typeof a == "string" || a instanceof Uint8Array) try {
				d = _.fromDER(a);
			} catch (e) {
				if (!(e instanceof wi.Err)) throw e;
				d = _.fromCompact(a);
			}
			else if (typeof a == "object" && typeof a.r == "bigint" && typeof a.s == "bigint") {
				let { r: e, s: t } = a;
				d = new _(e, t);
			} else throw Error("PARSE");
			f = l.fromHex(r);
		} catch (e) {
			if (e.message === "PARSE") throw Error("signature must be Signature instance, Uint8Array or hex string");
			return !1;
		}
		if (o && d.hasHighS()) return !1;
		u && (n = t.hash(n));
		let { r: p, s: m } = d, h = C(n), g = c(m), v = s(h * g), y = s(p * g), b = l.BASE.multiplyAndAddUnsafe(f, v, y)?.toAffine();
		return b ? s(b.x) === p : !1;
	}
	return {
		CURVE: t,
		getPublicKey: y,
		getSharedSecret: x,
		sign: k,
		verify: A,
		ProjectivePoint: l,
		Signature: _,
		utils: v
	};
}
//#endregion
//#region node_modules/@noble/curves/esm/_shortw_utils.js
function ji(e) {
	return {
		hash: e,
		hmac: (t, ...n) => Lt(e, t, At(...n)),
		randomBytes: Ft
	};
}
function Mi(e, t) {
	let n = (t) => Ai({
		...e,
		...ji(t)
	});
	return Object.freeze({
		...n(t),
		create: n
	});
}
//#endregion
//#region node_modules/@noble/curves/esm/secp256k1.js
var Ni = BigInt("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f"), Pi = BigInt("0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"), Fi = BigInt(1), Ii = BigInt(2), Li = (e, t) => (e + t / Ii) / t;
function Ri(e) {
	let t = Ni, n = BigInt(3), r = BigInt(6), i = BigInt(11), a = BigInt(22), o = BigInt(23), s = BigInt(44), c = BigInt(88), l = e * e * e % t, u = l * l * e % t, d = ni(ni(ni(u, n, t) * u % t, n, t) * u % t, Ii, t) * l % t, f = ni(d, i, t) * d % t, p = ni(f, a, t) * f % t, m = ni(p, s, t) * p % t, h = ni(ni(ni(ni(ni(ni(m, c, t) * m % t, s, t) * p % t, n, t) * u % t, o, t) * f % t, r, t) * l % t, Ii, t);
	if (!zi.eql(zi.sqr(h), e)) throw Error("Cannot find square root");
	return h;
}
var zi = fi(Ni, void 0, void 0, { sqrt: Ri }), Bi = Mi({
	a: BigInt(0),
	b: BigInt(7),
	Fp: zi,
	n: Pi,
	Gx: BigInt("55066263022277343669578718895168534326250603453777594175500187360389116729240"),
	Gy: BigInt("32670510020758816978083085130507043184471273380659243275938904335757337482424"),
	h: BigInt(1),
	lowS: !0,
	endo: {
		beta: BigInt("0x7ae96a2b657c07106e64479eac3434e99cf0497512f58995c1396c28719501ee"),
		splitScalar: (e) => {
			let t = Pi, n = BigInt("0x3086d221a7d46bcde86c90e49284eb15"), r = -Fi * BigInt("0xe4437ed6010e88286f547fa90abfe4c3"), i = BigInt("0x114ca50f7a8e2f3f657c1108d9d44cfd8"), a = n, o = BigInt("0x100000000000000000000000000000000"), s = Li(a * e, t), c = Li(-r * e, t), l = ei(e - s * n - c * i, t), u = ei(-s * r - c * a, t), d = l > o, f = u > o;
			if (d && (l = t - l), f && (u = t - u), l > o || u > o) throw Error("splitScalar: Endomorphism failed, k=" + e);
			return {
				k1neg: d,
				k1: l,
				k2neg: f,
				k2: u
			};
		}
	}
}, Yt);
Bi.ProjectivePoint;
//#endregion
//#region node_modules/ethers/lib.esm/constants/addresses.js
var Vi = "0x0000000000000000000000000000000000000000", Hi = "0x0000000000000000000000000000000000000000000000000000000000000000", Ui = BigInt(0), Wi = BigInt(1), Gi = BigInt(2), Ki = BigInt(27), qi = BigInt(28), Ji = BigInt(35), Yi = BigInt("0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"), Xi = Yi / Gi, Zi = Symbol.for("nodejs.util.inspect.custom"), Qi = {};
function $i(e) {
	return D(B(e), 32);
}
var ea = class e {
	#e;
	#t;
	#n;
	#r;
	get r() {
		return this.#e;
	}
	set r(e) {
		d(w(e) === 32, "invalid r", "value", e), this.#e = S(e);
	}
	get s() {
		return d(parseInt(this.#t.substring(0, 3)) < 8, "non-canonical s; use ._s", "s", this.#t), this.#t;
	}
	set s(e) {
		d(w(e) === 32, "invalid s", "value", e), this.#t = S(e);
	}
	get _s() {
		return this.#t;
	}
	isValid() {
		return BigInt(this.#t) <= Xi;
	}
	get v() {
		return this.#n;
	}
	set v(e) {
		let t = z(e, "value");
		d(t === 27 || t === 28, "invalid v", "v", e), this.#n = t;
	}
	get networkV() {
		return this.#r;
	}
	get legacyChainId() {
		let t = this.networkV;
		return t == null ? null : e.getChainId(t);
	}
	get yParity() {
		return this.v === 27 ? 0 : 1;
	}
	get yParityAndS() {
		let e = _(this.s);
		return this.yParity && (e[0] |= 128), S(e);
	}
	get compactSerialized() {
		return C([this.r, this.yParityAndS]);
	}
	get serialized() {
		return C([
			this.r,
			this.s,
			this.yParity ? "0x1c" : "0x1b"
		]);
	}
	constructor(e, t, n, r) {
		h(e, Qi, "Signature"), this.#e = t, this.#t = n, this.#n = r, this.#r = null;
	}
	getCanonical() {
		if (this.isValid()) return this;
		let t = Yi - BigInt(this._s), n = 55 - this.v, r = new e(Qi, this.r, $i(t), n);
		return this.networkV && (r.#r = this.networkV), r;
	}
	clone() {
		let t = new e(Qi, this.r, this._s, this.v);
		return this.networkV && (t.#r = this.networkV), t;
	}
	toJSON() {
		let e = this.networkV;
		return {
			_type: "signature",
			networkV: e == null ? null : e.toString(),
			r: this.r,
			s: this._s,
			v: this.v
		};
	}
	[Zi]() {
		return this.toString();
	}
	toString() {
		return this.isValid() ? `Signature { r: ${this.r}, s: ${this._s}, v: ${this.v} }` : `Signature { r: ${this.r}, s: ${this._s}, v: ${this.v}, valid: false }`;
	}
	static getChainId(e) {
		let t = F(e, "v");
		return t == Ki || t == qi ? Ui : (d(t >= Ji, "invalid EIP-155 v", "v", e), (t - Ji) / Gi);
	}
	static getChainIdV(e, t) {
		return F(e) * Gi + BigInt(35 + t - 27);
	}
	static getNormalizedV(e) {
		let t = F(e);
		return t === Ui || t === Ki ? 27 : t === Wi || t === qi ? 28 : (d(t >= Ji, "invalid v", "v", e), t & Wi ? 27 : 28);
	}
	static from(t) {
		function n(e, n) {
			d(e, n, "signature", t);
		}
		if (t == null) return new e(Qi, Hi, Hi, 27);
		if (typeof t == "string") {
			let r = _(t, "signature");
			if (r.length === 64) {
				let t = S(r.slice(0, 32)), n = r.slice(32, 64), i = n[0] & 128 ? 28 : 27;
				return n[0] &= 127, new e(Qi, t, S(n), i);
			}
			if (r.length === 65) {
				let t = S(r.slice(0, 32)), n = S(r.slice(32, 64)), i = e.getNormalizedV(r[64]);
				return new e(Qi, t, n, i);
			}
			n(!1, "invalid raw signature length");
		}
		if (t instanceof e) return t.clone();
		let r = t.r;
		n(r != null, "missing r");
		let i = $i(r), a = (function(e, t) {
			if (e != null) return $i(e);
			if (t != null) {
				n(y(t, 32), "invalid yParityAndS");
				let e = _(t);
				return e[0] &= 127, S(e);
			}
			n(!1, "missing s");
		})(t.s, t.yParityAndS), { networkV: o, v: s } = (function(t, r, i) {
			if (t != null) {
				let n = F(t);
				return {
					networkV: n >= Ji ? n : void 0,
					v: e.getNormalizedV(n)
				};
			}
			if (r != null) return n(y(r, 32), "invalid yParityAndS"), { v: _(r)[0] & 128 ? 28 : 27 };
			if (i != null) {
				switch (z(i, "sig.yParity")) {
					case 0: return { v: 27 };
					case 1: return { v: 28 };
				}
				n(!1, "invalid yParity");
			}
			n(!1, "missing v");
		})(t.v, t.yParityAndS, t.yParity), c = new e(Qi, i, a, s);
		return o && (c.#r = o), n(t.yParity == null || z(t.yParity, "sig.yParity") === c.yParity, "yParity mismatch"), n(t.yParityAndS == null || t.yParityAndS === c.yParityAndS, "yParityAndS mismatch"), c;
	}
}, ta = class e {
	#e;
	constructor(e) {
		d(w(e) === 32, "invalid private key", "privateKey", "[REDACTED]"), this.#e = S(e);
	}
	get privateKey() {
		return this.#e;
	}
	get publicKey() {
		return e.computePublicKey(this.#e);
	}
	get compressedPublicKey() {
		return e.computePublicKey(this.#e, !0);
	}
	sign(e) {
		d(w(e) === 32, "invalid digest length", "digest", e);
		let t = Bi.sign(v(e), v(this.#e), { lowS: !0 });
		return ea.from({
			r: te(t.r, 32),
			s: te(t.s, 32),
			v: t.recovery ? 28 : 27
		});
	}
	computeSharedSecret(t) {
		let n = e.computePublicKey(t);
		return S(Bi.getSharedSecret(v(this.#e), _(n), !1));
	}
	static computePublicKey(e, t) {
		let n = _(e, "key");
		if (n.length === 32) return S(Bi.getPublicKey(n, !!t));
		if (n.length === 64) {
			let e = /* @__PURE__ */ new Uint8Array(65);
			e[0] = 4, e.set(n, 1), n = e;
		}
		return S(Bi.ProjectivePoint.fromHex(n).toRawBytes(t));
	}
	static recoverPublicKey(e, t) {
		d(w(e) === 32, "invalid digest length", "digest", e);
		let n = ea.from(t), r = Bi.Signature.fromCompact(v(C([n.r, n.s])));
		r = r.addRecoveryBit(n.yParity);
		let i = r.recoverPublicKey(v(e));
		return d(i != null, "invalid signature for digest", "signature", t), "0x" + i.toHex(!1);
	}
	static addPoints(t, n, r) {
		let i = Bi.ProjectivePoint.fromHex(e.computePublicKey(t).substring(2)), a = Bi.ProjectivePoint.fromHex(e.computePublicKey(n).substring(2));
		return "0x" + i.add(a).toHex(!!r);
	}
}, na = BigInt(0), ra = BigInt(36);
function ia(e) {
	e = e.toLowerCase();
	let t = e.substring(2).split(""), n = /* @__PURE__ */ new Uint8Array(40);
	for (let e = 0; e < 40; e++) n[e] = t[e].charCodeAt(0);
	let r = _(W(n));
	for (let e = 0; e < 40; e += 2) r[e >> 1] >> 4 >= 8 && (t[e] = t[e].toUpperCase()), (r[e >> 1] & 15) >= 8 && (t[e + 1] = t[e + 1].toUpperCase());
	return "0x" + t.join("");
}
var aa = {};
for (let e = 0; e < 10; e++) aa[String(e)] = String(e);
for (let e = 0; e < 26; e++) aa[String.fromCharCode(65 + e)] = String(10 + e);
var oa = 15;
function sa(e) {
	e = e.toUpperCase(), e = e.substring(4) + e.substring(0, 2) + "00";
	let t = e.split("").map((e) => aa[e]).join("");
	for (; t.length >= oa;) {
		let e = t.substring(0, oa);
		t = parseInt(e, 10) % 97 + t.substring(e.length);
	}
	let n = String(98 - parseInt(t, 10) % 97);
	for (; n.length < 2;) n = "0" + n;
	return n;
}
var ca = (function() {
	let e = {};
	for (let t = 0; t < 36; t++) {
		let n = "0123456789abcdefghijklmnopqrstuvwxyz"[t];
		e[n] = BigInt(t);
	}
	return e;
})();
function la(e) {
	e = e.toLowerCase();
	let t = na;
	for (let n = 0; n < e.length; n++) t = t * ra + ca[e[n]];
	return t;
}
function K(e) {
	if (d(typeof e == "string", "invalid address", "address", e), e.match(/^(0x)?[0-9a-fA-F]{40}$/)) {
		e.startsWith("0x") || (e = "0x" + e);
		let t = ia(e);
		return d(!e.match(/([A-F].*[a-f])|([a-f].*[A-F])/) || t === e, "bad address checksum", "address", e), t;
	}
	if (e.match(/^XE[0-9]{2}[0-9A-Za-z]{30,31}$/)) {
		d(e.substring(2, 4) === sa(e), "bad icap checksum", "address", e);
		let t = la(e.substring(4)).toString(16);
		for (; t.length < 40;) t = "0" + t;
		return ia("0x" + t);
	}
	d(!1, "invalid address", "address", e);
}
//#endregion
//#region node_modules/ethers/lib.esm/address/contract-address.js
function ua(e) {
	let t = K(e.from), n = F(e.nonce, "tx.nonce").toString(16);
	return n = n === "0" ? "0x" : n.length % 2 ? "0x0" + n : "0x" + n, K(T(W(Qe([t, n])), 12));
}
//#endregion
//#region node_modules/ethers/lib.esm/address/checks.js
function da(e) {
	return e && typeof e.getAddress == "function";
}
async function fa(e, t) {
	let n = await t;
	return (n == null || n === "0x0000000000000000000000000000000000000000") && (u(typeof e != "string", "unconfigured name", "UNCONFIGURED_NAME", { value: e }), d(!1, "invalid AddressLike value; did not resolve to a value address", "target", e)), K(n);
}
function pa(e, t) {
	if (typeof e == "string") return e.match(/^0x[0-9a-f]{40}$/i) ? K(e) : (u(t != null, "ENS resolution requires a provider", "UNSUPPORTED_OPERATION", { operation: "resolveName" }), fa(e, t.resolveName(e)));
	if (da(e)) return fa(e, e.getAddress());
	if (e && typeof e.then == "function") return fa(e, e);
	d(!1, "unsupported addressable value", "target", e);
}
//#endregion
//#region node_modules/ethers/lib.esm/abi/typed.js
var ma = {};
function q(e, t) {
	let n = !1;
	return t < 0 && (n = !0, t *= -1), new ga(ma, `${n ? "" : "u"}int${t}`, e, {
		signed: n,
		width: t
	});
}
function J(e, t) {
	return new ga(ma, `bytes${t || ""}`, e, { size: t });
}
var ha = Symbol.for("_ethers_typed"), ga = class e {
	type;
	value;
	#e;
	_typedSymbol;
	constructor(e, t, n, r) {
		r ??= null, h(ma, e, "Typed"), a(this, {
			_typedSymbol: ha,
			type: t,
			value: n
		}), this.#e = r, this.format();
	}
	format() {
		if (this.type === "array" || this.type === "dynamicArray") throw Error("");
		return this.type === "tuple" ? `tuple(${this.value.map((e) => e.format()).join(",")})` : this.type;
	}
	defaultValue() {
		return 0;
	}
	minValue() {
		return 0;
	}
	maxValue() {
		return 0;
	}
	isBigInt() {
		return !!this.type.match(/^u?int[0-9]+$/);
	}
	isData() {
		return this.type.startsWith("bytes");
	}
	isString() {
		return this.type === "string";
	}
	get tupleName() {
		if (this.type !== "tuple") throw TypeError("not a tuple");
		return this.#e;
	}
	get arrayLength() {
		if (this.type !== "array") throw TypeError("not an array");
		return this.#e === !0 ? -1 : this.#e === !1 ? this.value.length : null;
	}
	static from(t, n) {
		return new e(ma, t, n);
	}
	static uint8(e) {
		return q(e, 8);
	}
	static uint16(e) {
		return q(e, 16);
	}
	static uint24(e) {
		return q(e, 24);
	}
	static uint32(e) {
		return q(e, 32);
	}
	static uint40(e) {
		return q(e, 40);
	}
	static uint48(e) {
		return q(e, 48);
	}
	static uint56(e) {
		return q(e, 56);
	}
	static uint64(e) {
		return q(e, 64);
	}
	static uint72(e) {
		return q(e, 72);
	}
	static uint80(e) {
		return q(e, 80);
	}
	static uint88(e) {
		return q(e, 88);
	}
	static uint96(e) {
		return q(e, 96);
	}
	static uint104(e) {
		return q(e, 104);
	}
	static uint112(e) {
		return q(e, 112);
	}
	static uint120(e) {
		return q(e, 120);
	}
	static uint128(e) {
		return q(e, 128);
	}
	static uint136(e) {
		return q(e, 136);
	}
	static uint144(e) {
		return q(e, 144);
	}
	static uint152(e) {
		return q(e, 152);
	}
	static uint160(e) {
		return q(e, 160);
	}
	static uint168(e) {
		return q(e, 168);
	}
	static uint176(e) {
		return q(e, 176);
	}
	static uint184(e) {
		return q(e, 184);
	}
	static uint192(e) {
		return q(e, 192);
	}
	static uint200(e) {
		return q(e, 200);
	}
	static uint208(e) {
		return q(e, 208);
	}
	static uint216(e) {
		return q(e, 216);
	}
	static uint224(e) {
		return q(e, 224);
	}
	static uint232(e) {
		return q(e, 232);
	}
	static uint240(e) {
		return q(e, 240);
	}
	static uint248(e) {
		return q(e, 248);
	}
	static uint256(e) {
		return q(e, 256);
	}
	static uint(e) {
		return q(e, 256);
	}
	static int8(e) {
		return q(e, -8);
	}
	static int16(e) {
		return q(e, -16);
	}
	static int24(e) {
		return q(e, -24);
	}
	static int32(e) {
		return q(e, -32);
	}
	static int40(e) {
		return q(e, -40);
	}
	static int48(e) {
		return q(e, -48);
	}
	static int56(e) {
		return q(e, -56);
	}
	static int64(e) {
		return q(e, -64);
	}
	static int72(e) {
		return q(e, -72);
	}
	static int80(e) {
		return q(e, -80);
	}
	static int88(e) {
		return q(e, -88);
	}
	static int96(e) {
		return q(e, -96);
	}
	static int104(e) {
		return q(e, -104);
	}
	static int112(e) {
		return q(e, -112);
	}
	static int120(e) {
		return q(e, -120);
	}
	static int128(e) {
		return q(e, -128);
	}
	static int136(e) {
		return q(e, -136);
	}
	static int144(e) {
		return q(e, -144);
	}
	static int152(e) {
		return q(e, -152);
	}
	static int160(e) {
		return q(e, -160);
	}
	static int168(e) {
		return q(e, -168);
	}
	static int176(e) {
		return q(e, -176);
	}
	static int184(e) {
		return q(e, -184);
	}
	static int192(e) {
		return q(e, -192);
	}
	static int200(e) {
		return q(e, -200);
	}
	static int208(e) {
		return q(e, -208);
	}
	static int216(e) {
		return q(e, -216);
	}
	static int224(e) {
		return q(e, -224);
	}
	static int232(e) {
		return q(e, -232);
	}
	static int240(e) {
		return q(e, -240);
	}
	static int248(e) {
		return q(e, -248);
	}
	static int256(e) {
		return q(e, -256);
	}
	static int(e) {
		return q(e, -256);
	}
	static bytes1(e) {
		return J(e, 1);
	}
	static bytes2(e) {
		return J(e, 2);
	}
	static bytes3(e) {
		return J(e, 3);
	}
	static bytes4(e) {
		return J(e, 4);
	}
	static bytes5(e) {
		return J(e, 5);
	}
	static bytes6(e) {
		return J(e, 6);
	}
	static bytes7(e) {
		return J(e, 7);
	}
	static bytes8(e) {
		return J(e, 8);
	}
	static bytes9(e) {
		return J(e, 9);
	}
	static bytes10(e) {
		return J(e, 10);
	}
	static bytes11(e) {
		return J(e, 11);
	}
	static bytes12(e) {
		return J(e, 12);
	}
	static bytes13(e) {
		return J(e, 13);
	}
	static bytes14(e) {
		return J(e, 14);
	}
	static bytes15(e) {
		return J(e, 15);
	}
	static bytes16(e) {
		return J(e, 16);
	}
	static bytes17(e) {
		return J(e, 17);
	}
	static bytes18(e) {
		return J(e, 18);
	}
	static bytes19(e) {
		return J(e, 19);
	}
	static bytes20(e) {
		return J(e, 20);
	}
	static bytes21(e) {
		return J(e, 21);
	}
	static bytes22(e) {
		return J(e, 22);
	}
	static bytes23(e) {
		return J(e, 23);
	}
	static bytes24(e) {
		return J(e, 24);
	}
	static bytes25(e) {
		return J(e, 25);
	}
	static bytes26(e) {
		return J(e, 26);
	}
	static bytes27(e) {
		return J(e, 27);
	}
	static bytes28(e) {
		return J(e, 28);
	}
	static bytes29(e) {
		return J(e, 29);
	}
	static bytes30(e) {
		return J(e, 30);
	}
	static bytes31(e) {
		return J(e, 31);
	}
	static bytes32(e) {
		return J(e, 32);
	}
	static address(t) {
		return new e(ma, "address", t);
	}
	static bool(t) {
		return new e(ma, "bool", !!t);
	}
	static bytes(t) {
		return new e(ma, "bytes", t);
	}
	static string(t) {
		return new e(ma, "string", t);
	}
	static array(e, t) {
		throw Error("not implemented yet");
	}
	static tuple(e, t) {
		throw Error("not implemented yet");
	}
	static overrides(t) {
		return new e(ma, "overrides", Object.assign({}, t));
	}
	static isTyped(e) {
		return e && typeof e == "object" && "_typedSymbol" in e && e._typedSymbol === ha;
	}
	static dereference(t, n) {
		if (e.isTyped(t)) {
			if (t.type !== n) throw Error(`invalid type: expecetd ${n}, got ${t.type}`);
			return t.value;
		}
		return t;
	}
}, _a = class extends pt {
	constructor(e) {
		super("address", "address", e, !1);
	}
	defaultValue() {
		return "0x0000000000000000000000000000000000000000";
	}
	encode(e, t) {
		let n = ga.dereference(t, "string");
		try {
			n = K(n);
		} catch (e) {
			return this._throwError(e.message, t);
		}
		return e.writeValue(n);
	}
	decode(e) {
		return K(te(e.readValue(), 20));
	}
}, va = class extends pt {
	coder;
	constructor(e) {
		super(e.name, e.type, "_", e.dynamic), this.coder = e;
	}
	defaultValue() {
		return this.coder.defaultValue();
	}
	encode(e, t) {
		return this.coder.encode(e, t);
	}
	decode(e) {
		return this.coder.decode(e);
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/abi/coders/array.js
function ya(e, t, n) {
	let r = [];
	if (Array.isArray(n)) r = n;
	else if (n && typeof n == "object") {
		let e = {};
		r = t.map((t) => {
			let r = t.localName;
			return u(r, "cannot encode object for signature with missing names", "INVALID_ARGUMENT", {
				argument: "values",
				info: { coder: t },
				value: n
			}), u(!e[r], "cannot encode object for signature with duplicate names", "INVALID_ARGUMENT", {
				argument: "values",
				info: { coder: t },
				value: n
			}), e[r] = !0, n[r];
		});
	} else d(!1, "invalid tuple value", "tuple", n);
	d(t.length === r.length, "types/value length mismatch", "tuple", n);
	let i = new mt(), a = new mt(), o = [];
	t.forEach((e, t) => {
		let n = r[t];
		if (e.dynamic) {
			let t = a.length;
			e.encode(a, n);
			let r = i.writeUpdatableValue();
			o.push((e) => {
				r(e + t);
			});
		} else e.encode(i, n);
	}), o.forEach((e) => {
		e(i.length);
	});
	let s = e.appendWriter(i);
	return s += e.appendWriter(a), s;
}
function ba(e, t) {
	let n = [], r = [], i = e.subReader(0);
	return t.forEach((t) => {
		let a = null;
		if (t.dynamic) {
			let n = e.readIndex(), r = i.subReader(n);
			try {
				a = t.decode(r);
			} catch (e) {
				if (s(e, "BUFFER_OVERRUN")) throw e;
				a = e, a.baseType = t.name, a.name = t.localName, a.type = t.type;
			}
		} else try {
			a = t.decode(e);
		} catch (e) {
			if (s(e, "BUFFER_OVERRUN")) throw e;
			a = e, a.baseType = t.name, a.name = t.localName, a.type = t.type;
		}
		if (a == null) throw Error("investigate");
		n.push(a), r.push(t.localName || null);
	}), dt.fromItems(n, r);
}
var xa = class extends pt {
	coder;
	length;
	constructor(e, t, n) {
		let r = e.type + "[" + (t >= 0 ? t : "") + "]", i = t === -1 || e.dynamic;
		super("array", r, n, i), a(this, {
			coder: e,
			length: t
		});
	}
	defaultValue() {
		let e = this.coder.defaultValue(), t = [];
		for (let n = 0; n < this.length; n++) t.push(e);
		return t;
	}
	encode(e, t) {
		let n = ga.dereference(t, "array");
		Array.isArray(n) || this._throwError("expected array value", n);
		let r = this.length;
		r === -1 && (r = n.length, e.writeValue(n.length)), f(n.length, r, "coder array" + (this.localName ? " " + this.localName : ""));
		let i = [];
		for (let e = 0; e < n.length; e++) i.push(this.coder);
		return ya(e, i, n);
	}
	decode(e) {
		let t = this.length;
		t === -1 && (t = e.readIndex(), u(t * 32 <= e.dataLength, "insufficient data length", "BUFFER_OVERRUN", {
			buffer: e.bytes,
			offset: t * 32,
			length: e.dataLength
		}));
		let n = [];
		for (let e = 0; e < t; e++) n.push(new va(this.coder));
		return ba(e, n);
	}
}, Sa = class extends pt {
	constructor(e) {
		super("bool", "bool", e, !1);
	}
	defaultValue() {
		return !1;
	}
	encode(e, t) {
		let n = ga.dereference(t, "bool");
		return e.writeValue(+!!n);
	}
	decode(e) {
		return !!e.readValue();
	}
}, Ca = class extends pt {
	constructor(e, t) {
		super(e, e, t, !0);
	}
	defaultValue() {
		return "0x";
	}
	encode(e, t) {
		t = v(t);
		let n = e.writeValue(t.length);
		return n += e.writeBytes(t), n;
	}
	decode(e) {
		return e.readBytes(e.readIndex(), !0);
	}
}, wa = class extends Ca {
	constructor(e) {
		super("bytes", e);
	}
	decode(e) {
		return S(super.decode(e));
	}
}, Ta = class extends pt {
	size;
	constructor(e, t) {
		let n = "bytes" + String(e);
		super(n, n, t, !1), a(this, { size: e }, { size: "number" });
	}
	defaultValue() {
		return "0x0000000000000000000000000000000000000000000000000000000000000000".substring(0, 2 + this.size * 2);
	}
	encode(e, t) {
		let n = v(ga.dereference(t, this.type));
		return n.length !== this.size && this._throwError("incorrect data length", t), e.writeBytes(n);
	}
	decode(e) {
		return S(e.readBytes(this.size));
	}
}, Ea = new Uint8Array([]), Da = class extends pt {
	constructor(e) {
		super("null", "", e, !1);
	}
	defaultValue() {
		return null;
	}
	encode(e, t) {
		return t != null && this._throwError("not null", t), e.writeBytes(Ea);
	}
	decode(e) {
		return e.readBytes(0), null;
	}
}, Oa = BigInt(0), ka = BigInt(1), Aa = BigInt("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), ja = class extends pt {
	size;
	signed;
	constructor(e, t, n) {
		let r = (t ? "int" : "uint") + e * 8;
		super(r, r, n, !1), a(this, {
			size: e,
			signed: t
		}, {
			size: "number",
			signed: "boolean"
		});
	}
	defaultValue() {
		return 0;
	}
	encode(e, t) {
		let n = F(ga.dereference(t, this.type)), r = P(Aa, 256);
		if (this.signed) {
			let e = P(r, this.size * 8 - 1);
			(n > e || n < -(e + ka)) && this._throwError("value out-of-bounds", t), n = N(n, 256);
		} else (n < Oa || n > P(r, this.size * 8)) && this._throwError("value out-of-bounds", t);
		return e.writeValue(n);
	}
	decode(e) {
		let t = P(e.readValue(), this.size * 8);
		return this.signed && (t = M(t, this.size * 8)), t;
	}
}, Ma = class extends Ca {
	constructor(e) {
		super("string", e);
	}
	defaultValue() {
		return "";
	}
	encode(e, t) {
		return super.encode(e, H(ga.dereference(t, "string")));
	}
	decode(e) {
		return me(super.decode(e));
	}
}, Na = class extends pt {
	coders;
	constructor(e, t) {
		let n = !1, r = [];
		e.forEach((e) => {
			e.dynamic && (n = !0), r.push(e.type);
		});
		let i = "tuple(" + r.join(",") + ")";
		super("tuple", i, t, n), a(this, { coders: Object.freeze(e.slice()) });
	}
	defaultValue() {
		let e = [];
		this.coders.forEach((t) => {
			e.push(t.defaultValue());
		});
		let t = this.coders.reduce((e, t) => {
			let n = t.localName;
			return n && (e[n] || (e[n] = 0), e[n]++), e;
		}, {});
		return this.coders.forEach((n, r) => {
			let i = n.localName;
			i && t[i] === 1 && (i === "length" && (i = "_length"), e[i] ?? (e[i] = e[r]));
		}), Object.freeze(e);
	}
	encode(e, t) {
		let n = ga.dereference(t, "tuple");
		return ya(e, this.coders, n);
	}
	decode(e) {
		return ba(e, this.coders);
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/transaction/accesslist.js
function Pa(e, t) {
	return {
		address: K(e),
		storageKeys: t.map((e, t) => (d(y(e, 32), "invalid slot", `storageKeys[${t}]`, e), e.toLowerCase()))
	};
}
function Fa(e) {
	if (Array.isArray(e)) return e.map((t, n) => Array.isArray(t) ? (d(t.length === 2, "invalid slot set", `value[${n}]`, t), Pa(t[0], t[1])) : (d(typeof t == "object" && !!t, "invalid address-slot set", "value", e), Pa(t.address, t.storageKeys)));
	d(typeof e == "object" && !!e, "invalid access list", "value", e);
	let t = Object.keys(e).map((t) => {
		let n = e[t].reduce((e, t) => (e[t] = !0, e), {});
		return Pa(t, Object.keys(n).sort());
	});
	return t.sort((e, t) => e.address.localeCompare(t.address)), t;
}
//#endregion
//#region node_modules/ethers/lib.esm/transaction/authorization.js
function Ia(e) {
	return {
		address: K(e.address),
		nonce: F(e.nonce == null ? 0 : e.nonce),
		chainId: F(e.chainId == null ? 0 : e.chainId),
		signature: ea.from(e.signature)
	};
}
//#endregion
//#region node_modules/ethers/lib.esm/transaction/address.js
function La(e) {
	let t;
	return t = typeof e == "string" ? ta.computePublicKey(e, !1) : e.publicKey, K(W("0x" + t.substring(4)).substring(26));
}
function Ra(e, t) {
	return La(ta.recoverPublicKey(e, t));
}
//#endregion
//#region node_modules/ethers/lib.esm/transaction/transaction.js
var za = BigInt(0), Ba = BigInt(2), Va = BigInt(27), Ha = BigInt(28), Ua = BigInt(35), Wa = BigInt("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), Ga = Symbol.for("nodejs.util.inspect.custom"), Ka = 131072, qa = 128;
function Ja(e) {
	return {
		blobToKzgCommitment: (t) => {
			if ("computeBlobProof" in e) {
				if ("blobToKzgCommitment" in e && typeof e.blobToKzgCommitment == "function") return _(e.blobToKzgCommitment(S(t)));
			} else if ("blobToKzgCommitment" in e && typeof e.blobToKzgCommitment == "function") return _(e.blobToKzgCommitment(t));
			if ("blobToKZGCommitment" in e && typeof e.blobToKZGCommitment == "function") return _(e.blobToKZGCommitment(S(t)));
			d(!1, "unsupported KZG library", "kzg", e);
		},
		computeBlobKzgProof: (t, n) => {
			if ("computeBlobProof" in e && typeof e.computeBlobProof == "function") return _(e.computeBlobProof(S(t), S(n)));
			if ("computeBlobKzgProof" in e && typeof e.computeBlobKzgProof == "function") return e.computeBlobKzgProof(t, n);
			if ("computeBlobKZGProof" in e && typeof e.computeBlobKZGProof == "function") return _(e.computeBlobKZGProof(S(t), S(n)));
			d(!1, "unsupported KZG library", "kzg", e);
		}
	};
}
function Ya(e, t) {
	let n = e.toString(16);
	for (; n.length < 2;) n = "0" + n;
	return n += _r(t).substring(4), "0x" + n;
}
function Xa(e) {
	return e === "0x" ? null : K(e);
}
function Za(e, t) {
	try {
		return Fa(e);
	} catch (n) {
		d(!1, n.message, t, e);
	}
}
function Qa(e, t) {
	try {
		if (!Array.isArray(e)) throw Error("authorizationList: invalid array");
		let t = [];
		for (let n = 0; n < e.length; n++) {
			let r = e[n];
			if (!Array.isArray(r)) throw Error(`authorization[${n}]: invalid array`);
			if (r.length !== 6) throw Error(`authorization[${n}]: wrong length`);
			if (!r[1]) throw Error(`authorization[${n}]: null address`);
			t.push({
				address: Xa(r[1]),
				nonce: Y(r[2], "nonce"),
				chainId: Y(r[0], "chainId"),
				signature: ea.from({
					yParity: $a(r[3], "yParity"),
					r: D(r[4], 32),
					s: D(r[5], 32)
				})
			});
		}
		return t;
	} catch (n) {
		d(!1, n.message, t, e);
	}
}
function $a(e, t) {
	return e === "0x" ? 0 : z(e, t);
}
function Y(e, t) {
	if (e === "0x") return za;
	let n = F(e, t);
	return d(n <= Wa, "value exceeds uint size", t, n), n;
}
function X(e, t) {
	let n = F(e, "value"), r = B(n);
	return d(r.length <= 32, "value too large", `tx.${t}`, n), r;
}
function eo(e) {
	return Fa(e).map((e) => [e.address, e.storageKeys]);
}
function to(e) {
	return e.map((e) => [
		X(e.chainId, "chainId"),
		e.address,
		X(e.nonce, "nonce"),
		X(e.signature.yParity, "yParity"),
		B(e.signature.r),
		B(e.signature._s)
	]);
}
function no(e, t) {
	d(Array.isArray(e), `invalid ${t}`, "value", e);
	for (let t = 0; t < e.length; t++) d(y(e[t], 32), "invalid ${ param } hash", `value[${t}]`, e[t]);
	return e;
}
function ro(e) {
	let t = Je(e);
	d(Array.isArray(t) && (t.length === 9 || t.length === 6), "invalid field count for legacy transaction", "data", e);
	let n = {
		type: 0,
		nonce: $a(t[0], "nonce"),
		gasPrice: Y(t[1], "gasPrice"),
		gasLimit: Y(t[2], "gasLimit"),
		to: Xa(t[3]),
		value: Y(t[4], "value"),
		data: S(t[5]),
		chainId: za
	};
	if (t.length === 6) return n;
	let r = Y(t[6], "v"), i = Y(t[7], "r"), a = Y(t[8], "s");
	if (i === za && a === za) n.chainId = r;
	else {
		let e = (r - Ua) / Ba;
		e < za && (e = za), n.chainId = e, d(e !== za || r === Va || r === Ha, "non-canonical legacy v", "v", t[6]), n.signature = ea.from({
			r: D(t[7], 32),
			s: D(t[8], 32),
			v: r
		});
	}
	return n;
}
function io(e, t) {
	let n = [
		X(e.nonce, "nonce"),
		X(e.gasPrice || 0, "gasPrice"),
		X(e.gasLimit, "gasLimit"),
		e.to || "0x",
		X(e.value, "value"),
		e.data
	], r = za;
	if (e.chainId != za) r = F(e.chainId, "tx.chainId"), d(!t || t.networkV == null || t.legacyChainId === r, "tx.chainId/sig.v mismatch", "sig", t);
	else if (e.signature) {
		let t = e.signature.legacyChainId;
		t != null && (r = t);
	}
	if (!t) return r !== za && (n.push(B(r)), n.push("0x"), n.push("0x")), Qe(n);
	let i = BigInt(27 + t.yParity);
	return r === za ? BigInt(t.v) !== i && d(!1, "tx.chainId/sig.v mismatch", "sig", t) : i = ea.getChainIdV(r, t.v), n.push(B(i)), n.push(B(t.r)), n.push(B(t._s)), Qe(n);
}
function ao(e, t) {
	let n;
	try {
		if (n = $a(t[0], "yParity"), n !== 0 && n !== 1) throw Error("bad yParity");
	} catch {
		d(!1, "invalid yParity", "yParity", t[0]);
	}
	let r = D(t[1], 32), i = D(t[2], 32);
	e.signature = ea.from({
		r,
		s: i,
		yParity: n
	});
}
function oo(e) {
	let t = Je(_(e).slice(1));
	d(Array.isArray(t) && (t.length === 9 || t.length === 12), "invalid field count for transaction type: 2", "data", S(e));
	let n = {
		type: 2,
		chainId: Y(t[0], "chainId"),
		nonce: $a(t[1], "nonce"),
		maxPriorityFeePerGas: Y(t[2], "maxPriorityFeePerGas"),
		maxFeePerGas: Y(t[3], "maxFeePerGas"),
		gasPrice: null,
		gasLimit: Y(t[4], "gasLimit"),
		to: Xa(t[5]),
		value: Y(t[6], "value"),
		data: S(t[7]),
		accessList: Za(t[8], "accessList")
	};
	return t.length === 9 || ao(n, t.slice(9)), n;
}
function so(e, t) {
	let n = [
		X(e.chainId, "chainId"),
		X(e.nonce, "nonce"),
		X(e.maxPriorityFeePerGas || 0, "maxPriorityFeePerGas"),
		X(e.maxFeePerGas || 0, "maxFeePerGas"),
		X(e.gasLimit, "gasLimit"),
		e.to || "0x",
		X(e.value, "value"),
		e.data,
		eo(e.accessList || [])
	];
	return t && (n.push(X(t.yParity, "yParity")), n.push(B(t.r)), n.push(B(t.s))), C(["0x02", Qe(n)]);
}
function co(e) {
	let t = Je(_(e).slice(1));
	d(Array.isArray(t) && (t.length === 8 || t.length === 11), "invalid field count for transaction type: 1", "data", S(e));
	let n = {
		type: 1,
		chainId: Y(t[0], "chainId"),
		nonce: $a(t[1], "nonce"),
		gasPrice: Y(t[2], "gasPrice"),
		gasLimit: Y(t[3], "gasLimit"),
		to: Xa(t[4]),
		value: Y(t[5], "value"),
		data: S(t[6]),
		accessList: Za(t[7], "accessList")
	};
	return t.length === 8 || ao(n, t.slice(8)), n;
}
function lo(e, t) {
	let n = [
		X(e.chainId, "chainId"),
		X(e.nonce, "nonce"),
		X(e.gasPrice || 0, "gasPrice"),
		X(e.gasLimit, "gasLimit"),
		e.to || "0x",
		X(e.value, "value"),
		e.data,
		eo(e.accessList || [])
	];
	return t && (n.push(X(t.yParity, "recoveryParam")), n.push(B(t.r)), n.push(B(t.s))), C(["0x01", Qe(n)]);
}
function uo(e) {
	let t = Je(_(e).slice(1)), n = "3", r = null, i = null;
	if (t.length === 4 && Array.isArray(t[0])) {
		n = "3 (network format)";
		let e = t[1], r = t[2], a = t[3];
		d(Array.isArray(e), "invalid network format: blobs not an array", "fields[1]", e), d(Array.isArray(r), "invalid network format: commitments not an array", "fields[2]", r), d(Array.isArray(a), "invalid network format: proofs not an array", "fields[3]", a), d(e.length === r.length, "invalid network format: blobs/commitments length mismatch", "fields", t), d(e.length === a.length, "invalid network format: blobs/proofs length mismatch", "fields", t), i = [];
		for (let n = 0; n < t[1].length; n++) i.push({
			data: e[n],
			commitment: r[n],
			proof: a[n]
		});
		t = t[0];
	} else if (t.length === 5 && Array.isArray(t[0])) {
		n = "3 (EIP-7594 network format)", r = z(t[1]);
		let e = t[2], a = t[3], o = t[4];
		d(r === 1, `unsupported EIP-7594 network format version: ${r}`, "fields[1]", r), d(Array.isArray(e), "invalid EIP-7594 network format: blobs not an array", "fields[2]", e), d(Array.isArray(a), "invalid EIP-7594 network format: commitments not an array", "fields[3]", a), d(Array.isArray(o), "invalid EIP-7594 network format: proofs not an array", "fields[4]", o), d(e.length === a.length, "invalid network format: blobs/commitments length mismatch", "fields", t), d(e.length * qa === o.length, "invalid network format: blobs/proofs length mismatch", "fields", t), i = [];
		for (let t = 0; t < e.length; t++) {
			let n = [];
			for (let e = 0; e < qa; e++) n.push(o[t * qa + e]);
			i.push({
				data: e[t],
				commitment: a[t],
				proof: C(n)
			});
		}
		t = t[0];
	}
	d(Array.isArray(t) && (t.length === 11 || t.length === 14), `invalid field count for transaction type: ${n}`, "data", S(e));
	let a = {
		type: 3,
		chainId: Y(t[0], "chainId"),
		nonce: $a(t[1], "nonce"),
		maxPriorityFeePerGas: Y(t[2], "maxPriorityFeePerGas"),
		maxFeePerGas: Y(t[3], "maxFeePerGas"),
		gasPrice: null,
		gasLimit: Y(t[4], "gasLimit"),
		to: Xa(t[5]),
		value: Y(t[6], "value"),
		data: S(t[7]),
		accessList: Za(t[8], "accessList"),
		maxFeePerBlobGas: Y(t[9], "maxFeePerBlobGas"),
		blobVersionedHashes: t[10],
		blobWrapperVersion: r
	};
	i && (a.blobs = i), d(a.to != null, `invalid address for transaction type: ${n}`, "data", e), d(Array.isArray(a.blobVersionedHashes), "invalid blobVersionedHashes: must be an array", "data", e);
	for (let t = 0; t < a.blobVersionedHashes.length; t++) d(y(a.blobVersionedHashes[t], 32), `invalid blobVersionedHash at index ${t}: must be length 32`, "data", e);
	return t.length === 11 || ao(a, t.slice(11)), a;
}
function fo(e, t, n) {
	let r = [
		X(e.chainId, "chainId"),
		X(e.nonce, "nonce"),
		X(e.maxPriorityFeePerGas || 0, "maxPriorityFeePerGas"),
		X(e.maxFeePerGas || 0, "maxFeePerGas"),
		X(e.gasLimit, "gasLimit"),
		e.to || "0x0000000000000000000000000000000000000000",
		X(e.value, "value"),
		e.data,
		eo(e.accessList || []),
		X(e.maxFeePerBlobGas || 0, "maxFeePerBlobGas"),
		no(e.blobVersionedHashes || [], "blobVersionedHashes")
	];
	if (t && (r.push(X(t.yParity, "yParity")), r.push(B(t.r)), r.push(B(t.s)), n)) {
		if (e.blobWrapperVersion != null) {
			let t = B(e.blobWrapperVersion), i = [];
			for (let { proof: e } of n) {
				let t = _(e), n = t.length / qa;
				for (let e = 0; e < t.length; e += n) i.push(t.subarray(e, e + n));
			}
			return C(["0x03", Qe([
				r,
				t,
				n.map((e) => e.data),
				n.map((e) => e.commitment),
				i
			])]);
		}
		return C(["0x03", Qe([
			r,
			n.map((e) => e.data),
			n.map((e) => e.commitment),
			n.map((e) => e.proof)
		])]);
	}
	return C(["0x03", Qe(r)]);
}
function po(e) {
	let t = Je(_(e).slice(1));
	d(Array.isArray(t) && (t.length === 10 || t.length === 13), "invalid field count for transaction type: 4", "data", S(e));
	let n = {
		type: 4,
		chainId: Y(t[0], "chainId"),
		nonce: $a(t[1], "nonce"),
		maxPriorityFeePerGas: Y(t[2], "maxPriorityFeePerGas"),
		maxFeePerGas: Y(t[3], "maxFeePerGas"),
		gasPrice: null,
		gasLimit: Y(t[4], "gasLimit"),
		to: Xa(t[5]),
		value: Y(t[6], "value"),
		data: S(t[7]),
		accessList: Za(t[8], "accessList"),
		authorizationList: Qa(t[9], "authorizationList")
	};
	return t.length === 10 || ao(n, t.slice(10)), n;
}
function mo(e, t) {
	let n = [
		X(e.chainId, "chainId"),
		X(e.nonce, "nonce"),
		X(e.maxPriorityFeePerGas || 0, "maxPriorityFeePerGas"),
		X(e.maxFeePerGas || 0, "maxFeePerGas"),
		X(e.gasLimit, "gasLimit"),
		e.to || "0x",
		X(e.value, "value"),
		e.data,
		eo(e.accessList || []),
		to(e.authorizationList || [])
	];
	return t && (n.push(X(t.yParity, "yParity")), n.push(B(t.r)), n.push(B(t.s))), C(["0x04", Qe(n)]);
}
var ho = class e {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	#o;
	#s;
	#c;
	#l;
	#u;
	#d;
	#f;
	#p;
	#m;
	#h;
	#g;
	#_;
	get type() {
		return this.#e;
	}
	set type(e) {
		switch (e) {
			case null:
				this.#e = null;
				break;
			case 0:
			case "legacy":
				this.#e = 0;
				break;
			case 1:
			case "berlin":
			case "eip-2930":
				this.#e = 1;
				break;
			case 2:
			case "london":
			case "eip-1559":
				this.#e = 2;
				break;
			case 3:
			case "cancun":
			case "eip-4844":
				this.#e = 3;
				break;
			case 4:
			case "pectra":
			case "eip-7702":
				this.#e = 4;
				break;
			default: d(!1, "unsupported transaction type", "type", e);
		}
	}
	get typeName() {
		switch (this.type) {
			case 0: return "legacy";
			case 1: return "eip-2930";
			case 2: return "eip-1559";
			case 3: return "eip-4844";
			case 4: return "eip-7702";
		}
		return null;
	}
	get to() {
		let e = this.#t;
		return e == null && this.type === 3 ? Vi : e;
	}
	set to(e) {
		this.#t = e == null ? null : K(e);
	}
	get nonce() {
		return this.#r;
	}
	set nonce(e) {
		this.#r = z(e, "value");
	}
	get gasLimit() {
		return this.#i;
	}
	set gasLimit(e) {
		this.#i = F(e);
	}
	get gasPrice() {
		let e = this.#a;
		return e == null && (this.type === 0 || this.type === 1) ? za : e;
	}
	set gasPrice(e) {
		this.#a = e == null ? null : F(e, "gasPrice");
	}
	get maxPriorityFeePerGas() {
		return this.#o ?? (this.type === 2 || this.type === 3 ? za : null);
	}
	set maxPriorityFeePerGas(e) {
		this.#o = e == null ? null : F(e, "maxPriorityFeePerGas");
	}
	get maxFeePerGas() {
		return this.#s ?? (this.type === 2 || this.type === 3 ? za : null);
	}
	set maxFeePerGas(e) {
		this.#s = e == null ? null : F(e, "maxFeePerGas");
	}
	get data() {
		return this.#n;
	}
	set data(e) {
		this.#n = S(e);
	}
	get value() {
		return this.#c;
	}
	set value(e) {
		this.#c = F(e, "value");
	}
	get chainId() {
		return this.#l;
	}
	set chainId(e) {
		this.#l = F(e);
	}
	get signature() {
		return this.#u || null;
	}
	set signature(e) {
		this.#u = e == null ? null : ea.from(e);
	}
	isValid() {
		let e = this.signature;
		if (e && !e.isValid()) return !1;
		let t = this.authorizationList;
		if (t) {
			for (let e of t) if (!e.signature.isValid()) return !1;
		}
		return !0;
	}
	get accessList() {
		return (this.#d || null) ?? (this.type === 1 || this.type === 2 || this.type === 3 ? [] : null);
	}
	set accessList(e) {
		this.#d = e == null ? null : Fa(e);
	}
	get authorizationList() {
		let e = this.#g || null;
		return e == null && this.type === 4 ? [] : e;
	}
	set authorizationList(e) {
		this.#g = e == null ? null : e.map((e) => Ia(e));
	}
	get maxFeePerBlobGas() {
		let e = this.#f;
		return e == null && this.type === 3 ? za : e;
	}
	set maxFeePerBlobGas(e) {
		this.#f = e == null ? null : F(e, "maxFeePerBlobGas");
	}
	get blobVersionedHashes() {
		let e = this.#p;
		return e == null && this.type === 3 ? [] : e;
	}
	set blobVersionedHashes(e) {
		if (e != null) {
			d(Array.isArray(e), "blobVersionedHashes must be an Array", "value", e), e = e.slice();
			for (let t = 0; t < e.length; t++) d(y(e[t], 32), "invalid blobVersionedHash", `value[${t}]`, e[t]);
		}
		this.#p = e;
	}
	get blobs() {
		return this.#h == null ? null : this.#h.map((e) => Object.assign({}, e));
	}
	set blobs(e) {
		if (e == null) {
			this.#h = null;
			return;
		}
		let t = [], n = [];
		for (let r = 0; r < e.length; r++) {
			let i = e[r];
			if (b(i)) {
				u(this.#m, "adding a raw blob requires a KZG library", "UNSUPPORTED_OPERATION", { operation: "set blobs()" });
				let e = _(i);
				if (d(e.length <= Ka, "blob is too large", `blobs[${r}]`, i), e.length !== Ka) {
					let t = new Uint8Array(Ka);
					t.set(e), e = t;
				}
				let a = this.#m.blobToKzgCommitment(e), o = S(this.#m.computeBlobKzgProof(e, a));
				t.push({
					data: S(e),
					commitment: S(a),
					proof: o
				}), n.push(Ya(1, a));
			} else {
				let e = S(i.data), r = S(i.commitment), a = S(i.proof);
				t.push({
					data: e,
					commitment: r,
					proof: a
				}), n.push(Ya(1, r));
			}
		}
		this.#h = t, this.#p = n;
	}
	get kzg() {
		return this.#m;
	}
	set kzg(e) {
		this.#m = e == null ? null : Ja(e);
	}
	get blobWrapperVersion() {
		return this.#_;
	}
	set blobWrapperVersion(e) {
		this.#_ = e;
	}
	constructor() {
		this.#e = null, this.#t = null, this.#r = 0, this.#i = za, this.#a = null, this.#o = null, this.#s = null, this.#n = "0x", this.#c = za, this.#l = za, this.#u = null, this.#d = null, this.#f = null, this.#p = null, this.#m = null, this.#h = null, this.#g = null, this.#_ = null;
	}
	get hash() {
		return this.signature == null ? null : W(this.#v(!0, !1));
	}
	get unsignedHash() {
		return W(this.unsignedSerialized);
	}
	get from() {
		return this.signature == null ? null : Ra(this.unsignedHash, this.signature.getCanonical());
	}
	get fromPublicKey() {
		return this.signature == null ? null : ta.recoverPublicKey(this.unsignedHash, this.signature.getCanonical());
	}
	isSigned() {
		return this.signature != null;
	}
	#v(e, t) {
		u(!e || this.signature != null, "cannot serialize unsigned transaction; maybe you meant .unsignedSerialized", "UNSUPPORTED_OPERATION", { operation: ".serialized" });
		let n = e ? this.signature : null;
		switch (this.inferType()) {
			case 0: return io(this, n);
			case 1: return lo(this, n);
			case 2: return so(this, n);
			case 3: return fo(this, n, t ? this.blobs : null);
			case 4: return mo(this, n);
		}
		u(!1, "unsupported transaction type", "UNSUPPORTED_OPERATION", { operation: ".serialized" });
	}
	get serialized() {
		return this.#v(!0, !0);
	}
	get unsignedSerialized() {
		return this.#v(!1, !1);
	}
	inferType() {
		let e = this.inferTypes();
		return e.indexOf(2) >= 0 ? 2 : e.pop();
	}
	inferTypes() {
		let e = this.gasPrice != null, t = this.maxFeePerGas != null || this.maxPriorityFeePerGas != null, n = this.accessList != null, r = this.#f != null || this.#p;
		this.maxFeePerGas != null && this.maxPriorityFeePerGas != null && u(this.maxFeePerGas >= this.maxPriorityFeePerGas, "priorityFee cannot be more than maxFee", "BAD_DATA", { value: this }), u(!t || this.type !== 0 && this.type !== 1, "transaction type cannot have maxFeePerGas or maxPriorityFeePerGas", "BAD_DATA", { value: this }), u(this.type !== 0 || !n, "legacy transaction cannot have accessList", "BAD_DATA", { value: this });
		let i = [];
		return this.type == null ? this.authorizationList && this.authorizationList.length ? i.push(4) : t ? i.push(2) : e ? (i.push(1), n || i.push(0)) : n ? (i.push(1), i.push(2)) : r && this.to ? i.push(3) : (i.push(0), i.push(1), i.push(2), i.push(3)) : i.push(this.type), i.sort(), i;
	}
	isLegacy() {
		return this.type === 0;
	}
	isBerlin() {
		return this.type === 1;
	}
	isLondon() {
		return this.type === 2;
	}
	isCancun() {
		return this.type === 3;
	}
	clone() {
		return e.from(this);
	}
	toJSON() {
		let e = (e) => e == null ? null : e.toString();
		return {
			type: this.type,
			to: this.to,
			data: this.data,
			nonce: this.nonce,
			gasLimit: e(this.gasLimit),
			gasPrice: e(this.gasPrice),
			maxPriorityFeePerGas: e(this.maxPriorityFeePerGas),
			maxFeePerGas: e(this.maxFeePerGas),
			value: e(this.value),
			chainId: e(this.chainId),
			sig: this.signature ? this.signature.toJSON() : null,
			accessList: this.accessList
		};
	}
	[Ga]() {
		return this.toString();
	}
	toString() {
		let e = [], t = (t) => {
			let n = this[t];
			typeof n == "string" && (n = JSON.stringify(n)), e.push(`${t}: ${n}`);
		};
		this.type && t("type"), t("to"), t("data"), t("nonce"), t("gasLimit"), t("value"), this.chainId != null && t("chainId"), this.signature && (t("from"), e.push(`signature: ${this.signature.toString()}`));
		let n = this.authorizationList;
		if (n) {
			let t = [];
			for (let e of n) {
				let n = [];
				n.push(`address: ${JSON.stringify(e.address)}`), e.nonce != null && n.push(`nonce: ${e.nonce}`), e.chainId != null && n.push(`chainId: ${e.chainId}`), e.signature && n.push(`signature: ${e.signature.toString()}`), t.push(`Authorization { ${n.join(", ")} }`);
			}
			e.push(`authorizations: [ ${t.join(", ")} ]`);
		}
		return `Transaction { ${e.join(", ")} }`;
	}
	static from(t) {
		if (t == null) return new e();
		if (typeof t == "string") {
			let n = _(t);
			if (n[0] >= 127) return e.from(ro(n));
			switch (n[0]) {
				case 1: return e.from(co(n));
				case 2: return e.from(oo(n));
				case 3: return e.from(uo(n));
				case 4: return e.from(po(n));
			}
			u(!1, "unsupported transaction type", "UNSUPPORTED_OPERATION", { operation: "from" });
		}
		let n = new e();
		return t.type != null && (n.type = t.type), t.to != null && (n.to = t.to), t.nonce != null && (n.nonce = t.nonce), t.gasLimit != null && (n.gasLimit = t.gasLimit), t.gasPrice != null && (n.gasPrice = t.gasPrice), t.maxPriorityFeePerGas != null && (n.maxPriorityFeePerGas = t.maxPriorityFeePerGas), t.maxFeePerGas != null && (n.maxFeePerGas = t.maxFeePerGas), t.maxFeePerBlobGas != null && (n.maxFeePerBlobGas = t.maxFeePerBlobGas), t.data != null && (n.data = t.data), t.value != null && (n.value = t.value), t.chainId != null && (n.chainId = t.chainId), t.signature != null && (n.signature = ea.from(t.signature)), t.accessList != null && (n.accessList = t.accessList), t.authorizationList != null && (n.authorizationList = t.authorizationList), t.blobVersionedHashes != null && (n.blobVersionedHashes = t.blobVersionedHashes), t.kzg != null && (n.kzg = t.kzg), t.blobWrapperVersion != null && (n.blobWrapperVersion = t.blobWrapperVersion), t.blobs != null && (n.blobs = t.blobs), t.hash != null && (d(n.isSigned(), "unsigned transaction cannot define '.hash'", "tx", t), d(n.hash === t.hash, "hash mismatch", "tx", t)), t.from != null && (d(n.isSigned(), "unsigned transaction cannot define '.from'", "tx", t), d(n.from.toLowerCase() === (t.from || "").toLowerCase(), "from mismatch", "tx", t)), n;
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/hash/id.js
function go(e) {
	return W(H(e));
}
//#endregion
//#region node_modules/@adraffy/ens-normalize/dist/index.mjs
var _o = "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", vo = /* @__PURE__ */ new Map([
	[8217, "apostrophe"],
	[8260, "fraction slash"],
	[12539, "middle dot"]
]), yo = 4;
function bo(e) {
	let t = 0;
	function n() {
		return e[t++] << 8 | e[t++];
	}
	let r = n(), i = 1, a = [0, 1];
	for (let e = 1; e < r; e++) a.push(i += n());
	let o = n(), s = t;
	t += o;
	let c = 0, l = 0;
	function u() {
		return c == 0 && (l = l << 8 | e[t++], c = 8), l >> --c & 1;
	}
	let d = 2 ** 31, f = d >>> 1, p = f >> 1, m = d - 1, h = 0;
	for (let e = 0; e < 31; e++) h = h << 1 | u();
	let g = [], _ = 0, v = d;
	for (;;) {
		let e = Math.floor(((h - _ + 1) * i - 1) / v), t = 0, n = r;
		for (; n - t > 1;) {
			let r = t + n >>> 1;
			e < a[r] ? n = r : t = r;
		}
		if (t == 0) break;
		g.push(t);
		let o = _ + Math.floor(v * a[t] / i), s = _ + Math.floor(v * a[t + 1] / i) - 1;
		for (; ((o ^ s) & f) == 0;) h = h << 1 & m | u(), o = o << 1 & m, s = s << 1 & m | 1;
		for (; o & ~s & p;) h = h & f | h << 1 & m >>> 1 | u(), o = o << 1 ^ f, s = (s ^ f) << 1 | f | 1;
		_ = o, v = 1 + s - o;
	}
	let y = r - 4;
	return g.map((t) => {
		switch (t - y) {
			case 3: return y + 65792 + (e[s++] << 16 | e[s++] << 8 | e[s++]);
			case 2: return y + 256 + (e[s++] << 8 | e[s++]);
			case 1: return y + e[s++];
			default: return t - 1;
		}
	});
}
function xo(e) {
	let t = 0;
	return () => e[t++];
}
function So(e) {
	return xo(bo(Co(e)));
}
function Co(e) {
	let t = [];
	[..."ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"].forEach((e, n) => t[e.charCodeAt(0)] = n);
	let n = e.length, r = new Uint8Array(6 * n >> 3);
	for (let i = 0, a = 0, o = 0, s = 0; i < n; i++) s = s << 6 | t[e.charCodeAt(i)], o += 6, o >= 8 && (r[a++] = s >> (o -= 8));
	return r;
}
function wo(e) {
	return e & 1 ? ~e >> 1 : e >> 1;
}
function To(e, t) {
	let n = Array(e);
	for (let r = 0, i = 0; r < e; r++) n[r] = i += wo(t());
	return n;
}
function Eo(e, t = 0) {
	let n = [];
	for (;;) {
		let r = e(), i = e();
		if (!i) break;
		t += r;
		for (let e = 0; e < i; e++) n.push(t + e);
		t += i + 1;
	}
	return n;
}
function Do(e) {
	return ko(() => {
		let t = Eo(e);
		if (t.length) return t;
	});
}
function Oo(e) {
	let t = [];
	for (;;) {
		let n = e();
		if (n == 0) break;
		t.push(jo(n, e));
	}
	for (;;) {
		let n = e() - 1;
		if (n < 0) break;
		t.push(Mo(n, e));
	}
	return t.flat();
}
function ko(e) {
	let t = [];
	for (;;) {
		let n = e(t.length);
		if (!n) break;
		t.push(n);
	}
	return t;
}
function Ao(e, t, n) {
	let r = Array(e).fill().map(() => []);
	for (let i = 0; i < t; i++) To(e, n).forEach((e, t) => r[t].push(e));
	return r;
}
function jo(e, t) {
	let n = 1 + t(), r = t(), i = ko(t);
	return Ao(i.length, 1 + e, t).flatMap((e, t) => {
		let [a, ...o] = e;
		return Array(i[t]).fill().map((e, t) => {
			let i = t * r;
			return [a + t * n, o.map((e) => e + i)];
		});
	});
}
function Mo(e, t) {
	return Ao(1 + t(), 1 + e, t).map((e) => [e[0], e.slice(1)]);
}
function No(e) {
	let t = [], n = Eo(e);
	return i(r([]), []), t;
	function r(t) {
		return {
			S: e(),
			B: ko(() => {
				let t = Eo(e).map((e) => n[e]);
				if (t.length) return r(t);
			}),
			Q: t
		};
	}
	function i({ S: e, B: n }, r, a) {
		if (!(e & 4 && a === r[r.length - 1])) {
			e & 2 && (a = r[r.length - 1]), e & 1 && t.push(r);
			for (let e of n) for (let t of e.Q) i(e, [...r, t], a);
		}
	}
}
function Po(e) {
	return e.toString(16).toUpperCase().padStart(2, "0");
}
function Fo(e) {
	return `{${Po(e)}}`;
}
function Io(e) {
	let t = [];
	for (let n = 0, r = e.length; n < r;) {
		let r = e.codePointAt(n);
		n += r < 65536 ? 1 : 2, t.push(r);
	}
	return t;
}
function Lo(e) {
	let t = 4096, n = e.length;
	if (n < t) return String.fromCodePoint(...e);
	let r = [];
	for (let i = 0; i < n;) r.push(String.fromCodePoint(...e.slice(i, i += t)));
	return r.join("");
}
function Ro(e, t) {
	let n = e.length, r = n - t.length;
	for (let i = 0; r == 0 && i < n; i++) r = e[i] - t[i];
	return r;
}
var zo = "AEUDWAHSCGYATwDVADIAdgAiADQAFAAtABQAIQAPACcADQASAAsAGQAJABIACQARAAUACwAFAAwABQAQAAMABwAEAAoABQAJAAIACgABAAQAFAALAAIACwABAAIAAQAHAAMAAwAEAAsADAAMAAwACwANAA0AAwAKAAkABAAdAAYAZwDTAecDNACxCmIB8xhZAqfoC190UGcThgBurwf7PT09Pb09AjgJum8OjDllxHYUKXAPxzq6tABAxgK8ysUvWAgMPT09PT09PSs6LT2HcgWXWwFLoSMEEEl5RFVMKvO0XQ8ExDdJMnIgPi89uj00MsvBXxEPAGPCDwBnQKoEbwRwBHEEcgRzBHQEdQR2BHcEeAR6BHsEfAR+BIAEgfndBQoBYgULAWIFDAFiBNcE2ATZBRAFEQUvBdALFAsVDPcNBw13DYcOMA4xDjMB4BllHI0B2grbAMDpHLkQ7QHVAPRNQQFnGRUEg0yEB2uaJEMAJpIBpob5AERSMAKNoAXqaQLRBMCzEiC+AZ4EWRJJFbEu7QDQLARtEbgECxDwAb/RyAk1AV4nD2cEQQKTAzsAGpobPgAahAGPCrysdy0OAKwAfFIcBAQFUmoA/PtZADkBIadVj2UMUgx5Il4ANQC9vLIBDAHUGVsQ8wCzfQIbGVcCHBZHAZ8CBAgXOhG7AqMZ4M7+1M0UAPDNAWsC+mcJDe8AAQA99zkEXLICyQozAo6lAobcP5JvjQLFzwKD9gU/OD8FEQCtEQL6bW+nAKUEvzjDHsuRyUvOFHcacUz5AqIFRSE2kzsBEQCuaQL5DQTlcgO6twSpTiUgCwIFCAUXBHQEqQV6swAVxUlmTmsCwjqsP/wKJQmXb793UgZBEBsnpRD3DDMBtQE7De1L2ATxBjsEyR99GRkPzZWcCKUt3QztJuMuoYBaI/UqgwXtS/Q83QtNUWgPWQtlCeM6Y4FOAyEBDSKLCt0NOQhtEPMKyWsN5RFFBzkD1UmaAKUHAQsRHTUVtSYQYqwLCTl3Bvsa9guPJq8TKXr8BdMaIQZNASka/wDPLueFsFoxXBxPXwYDCyUjxxSoUCANJUC3eEgaGwcVJakCkUNwSodRNh6TIfY8PQ1mLhNRfAf1PAUZTwuBPJ5Gq0UOEdI+jT1IIklMLAQ1fywvJ4sJzw+FDLl8cgFZCSEJsQxxEzERFzfFCDkHGS2XJCcVCCFGlWCaBPefA/MT0QMLBT8JQQcTA7UcLRMuFSkFDYEk1wLzNtUuswKPVoABFwXLDyUf3xBQR+AO6QibAmUDgyXrAC0VIQAXIpsIQ2MAX4/YUwUuywjHamwjdANnFOdhEXMHkQ5XB6ccMxW/HOFwyF4Lhggoo68JWwF1CZkBXwTjCAk1W4ygIEFnU4tYGJsgYUE/XfwCMQxlFZ9EvYd4AosPaxIbATUBcwc5DQECdxHtEWsQlQjrhgQ1tTP4OiUETyGDIBEKJwNPbM4LJyb5DPhpAaMSYgMMND137merYLYkF/0HGTLFQWAh8QuST80MnBrBGEJULhnkB78D8xrzJ+pBVwX/A6MDEzpNM+4EvQtpCIsJPwBJDqMXB9cYagpxjNABMYsBt5kDV5GDAm+PBjcHCwBnC4cFeeUAHQKnCKMABQDPA1cAOQKtB50AGQCFQQE9AycvASHlAo8DkwgxywGVLwHzKQQbwwwVAPc3bkoCw7ECgGpmogXdWAKOAkk1AU0lBAVOR1EDr3HhANsASwYT30cBFatKyxrjQwHfbysAxwD7AAU1BwVBAc0B820AtwFfCzEJorO1AU3pKQCDABVrAdcCiQDdADUAf/EBUwBNBVn5BdMCT0kBETEYK1dhAbsDHwEzAQ0AeQbLjaXJBx8EbQfTAhAbFeEC7y4HtQEDIt8TzULFAr3eVaFgAmSBAmJCW02vWzcgAqH3AmiYAmYJAp+EOBsLAmY7AmYmBG4EfwN/EwN+kjkGOXcXOYI6IyMCbB0CMjY4CgJtxwJtru+KM2dFKwFnAN4A4QBKBQeYDI0A/gvCAA21AncvAnaiPwJ5S0MCeLodXNtFrkbXAnw/AnrIAn0JAnzwBVkFIEgASH1jJAKBbQKAAAKABQJ/rklYSlsVF0rMAtEBAtDMSycDiE8Dh+ZExZEyAvKhXQMDA65LzkwtJQPPTUxNrwKLPwKK2MEbBx1DZwW3Ao43Ao5cQJeBAo7ZAo5ceFG0UzUKUtRUhQKT+wKTDADpABxVHlWvVdAGLBsplYYy4XhmRTs5ApefAu+yWCGoAFklApaPApZ8nACpWaxaCYFNADsClrUClk5cRFzRApnLAplkXMpdBxkCnJs5wjqdApwWAp+bAp64igAdDzEqDwKd8QKekgC1PWE0Ye8CntMCoG4BqQKenx8Cnk6lY8hkJyUrAievAiZ+AqD7AqBMAqLdAqHEAqYvAqXOAqf/AH0Cp/JofGixAANJahxq0QKs4wKsrgKtZwKtAgJXHQJV3AKx4dcDH05slwKyvQ0CsugXbOBtY21IXwMlzQK2XDs/bpADKUUCuF4CuUcVArkqd3A2cOECvRkCu9pwlgMyEQK+iHICAzNxAr4acyJzTwLDywLDBHOCdEs1RXTgAzynAzyaAz2/AsV8AsZHAsYQiQLIaVECyEQCyU8CyS4CZJ0C3dJ4eWF4rnklS9ADGKNnAgJh9BnzlSR7C16SXrsRAs9rAs9sL0tT0vMTnwDGrQLPcwEp6gNOEn5LBQLcJwLbigLSTwNSXANTXwEBA1WMgIk/AMsW7WBFghyC04LOg40C2scC2d6EEIRJpzwDhqUALwNkDoZxWfkAVQLfZQLeuHN3AuIv7RQB8zAnAfSbAfLShwLr8wLpcHkC6vkC6uQA+UcBuQLuiQLrnJaqlwMC7j8DheCYeXDgcaEC8wMAaQOOFpmTAvcTA5FuA5KHAveYAvnZAvhmmhyaq7s3mx4DnYMC/voBGwA5nxyfswMFjQOmagOm2QDRxQMGaqGIogUJAwxJAtQAPwMA4UEXUwER8wNrB5dnBQCTLSu3r73bAYmZFH8RBDkB+ykFIQ6dCZ8Akv0TtRQrxQL3LScApQC3BbmOkRc/xqdtQS4UJo0uAUMBgPwBtSYAdQMOBG0ALAIWDKEAAAoCPQJqA90DfgSRASBFBSF8CgAFAEQAEwA2EgJ3AQAF1QNr7wrFAgD3Cp8nv7G35QGRIUFCAekUfxE0wIkABAAbAFoCRQKEiwAGOlM6lI1tALg6jzrQAI04wTrcAKUA6ADLATqBOjs5/Dn5O3aJOls7nok6bzkYAVYBMwFsBS81XTWeNa01ZjV1NbY1xTWCNZE10jXhNZ41rTXuNf01sjXBNgI2ETXGNdU2FjYnNd417TYuNj02LjUtITY6Nj02PDbJNwgEkDxXNjg23TcgNw82yiA3iTcwCgSwPGc2JDcZN2w6jTchQtRDB0LgQwscDw8JmyhtKFFVBgDpfwDpsAD+mxQ91wLpNSMArQC9BbeOkRdLxptzBL8MDAMMAQgDAAkKCwsLCQoGBAVVBI/DvwDz9b29kaUCb0QtsRTNLt4eGBcSHAMZFhYZEhYEARAEBUEcQRxBHEEcQRxBHEEaQRxBHEFCSTxBPElISUhBNkM2QTYbNklISVmBVIgELgEaJZkC7aMAoQCjBcGOmxdNxrsBvwGJAaQcEZ0ePCklMAAhMvAIMAL54gC7Bm8EescjzQMpARQpKgDUHqSvAj5Gqwr7YrMUACT9AN3rpF27H7fsd/twPt4l+UW1yQYKBt2Cgy7qJpGiLcdE2P1cQSImUbqJ6ICH27H4knQMIRMrFkHu3sx6tC35Y+eLIh4e4CMKJ4DfyV+8mfta499RCAJ0xfeZR8PsoYOApva9pjGn4PhvyZS7/h5JLuhaucfjuU+Z584wwqNO4hWYmaBCcjgQPale1bjoHzMUbut/zTgxHxBnAyrdKpF4IRMASLBtD/jviyLeCgj8twWjAd3HchN/uqaeRYeHJgl7JEY9/cTrvtfybx/r3Y/NtxJ9dp+MTVmiS9bwBH73s8Di56/Ma+mTPMHq4T1yEG1fWcqr0u+hrGnJEvU1JJAm/maQSrKrazIyvSkDFkj8UUlfBq8baniTGPng6YZRL661rDNw4w/1g2figG0IhXnL7wosd/sVNo5dYSmMBTP5c7rYLjRdCwg8quwljOMPf63D8ICAL0r71XRiyFHdgwHbwfgnPOf4Lzjf2v+j+IiDHG2isp5yUnzSDyDRb4i/Vs0qHSHq8PiEQ/JnBP7PxnjN0j6gT4AVAeRx/1o9VnEUlUwvFrzJqHk9jxAw4sYxCnrxaeBdCFFKbnE7z+x54F5W7ZZsU6kx8Qocul6FoAHHy01FGL/nne61mn4+uYXfQ1Uccn+HMLKE+cZzT8BB1E3FRskOgJrRsq25rauLm8+uamXpkS/bTy6y1wDbCrW4eD532kTWrtNUmVVZOIn/C+/JR9KVR5iG9TY8iaT67ubm/whL1xbKZoqtY+a6fNxMJrg211bGYJDUkYMNWA0BMB++9zOm6Eik4roqs9CCEFW0lyAK0PbvlzvoxrZuY/OEhNW/l/63U15Od/RSvmDvXpGLiVmeGi5PDSH2bYz5o2g6wFDQ2FbZgYgTF8rPlvA1ifjZD3NLtFdXdpSIJvgKR7GpjJWG7GZGawPomIH8B5tUmtHH9LpM+/KQKunEPa1GiQkCXv4Cnm9DLORo2joicHdPDZ64obQrPZ5bgqckkj0G6/NEiPYBY4bCkL7W8G5YzsUb6GakFjykSPkT7JGeLeB6uJOGMm+x7N381BCDfbJFx0dtLgV9Q477BfL1fvitX5anV/oYfxeYl+eF5x5bB8+Ep/L2nsmd56aKF4aAD4GbJWsdKyBW22xEmAD3XdbtsMyAFoR5mOla0gEd9U/YVB7zvHGpHbQonay9Sv0bQ8iZ8piaXVrKc5AG1AmqqgaEvzHSP2Wux7aZTWh6quVDVU01JtMIVRdCFwlSbbqqhoFlyzsotQzRexFvZ/MqUSFu3OhRIuNBbufvBpdVgb8XdGJ48/lJPCZ7dsOujTTbKPSEvGXkOnG2Xdi8/nM3EMRqITd5QeU7iOjKqC7URJY6TnLsHij22xAHKnVRD5MDtBYnoGFqZGMDmXCW6Oj+BAWw14hESY/xLF6bLku06AHkiXTHPCFZ0f9YSqqo27eAhhS67OrA2Het4M9JM3jm/yRX6bYxnfmzYl5qQdHxN08FsNuWDrWd4vMUY2QD3hr8vS73SCTkFoXZR3xNzOQt8d/6HfjBmXqvrE6EGkLzK6YK2U2/ksU/iUH+LvVIsJI+ri2AL/klo+ShdDyfs5A83i2prkMs51IKR7ZcqjZJi5X3+bd8GlyWvtddxKEoEqSgEO7A8jIgf2nH0h8FjM7oB6yte3X5mpL0i/E4Rx0CotKnILJj/vJqo4VkPQ93jRtRVfaitQPqldl5xRYPq8387Z0DcnZvOeION0Ht1+P27kFLGQIcLBX4FG3sffccNHh5cPfzp9INoRtqVtdViJfg8RjnXiIz/MNqEN6zvzX3hMzyWC7oSoXIT14ubc0abPX8Rp9GVa5NI/8iv+6ela1oTncbdimRKnrbRffDR/X4nH+bgqAuHWl7hOaeXPWVzIeRl7ga+JzD4Sx3mlj/q6Ra/E2HhDf21eEzTLNGfCZsY+/yxZzQzIAuijG65ii4O/waAJCrEJaWd/DRAKMQ5678Dw5AT7RCKzdadIwd8LsD+DgPBASmWsUlf8R0k1w/2k4lO2Wpb4zMI6EJVJs0xk/wn8/fRUPqrDKhbjHR41SqgFMx5RGMPuduFwlu5lK89tW11sTqiX/5EfGs5nO+y9FKvgXKPOEmgE05EKNL6Sjb3xS40H3BVPhm0ESOZgAjZoymc8be0inDVo4JdJVf+NKd3tN/CaB7GShhH27qf95NoFZVX/6ZkR2lX+CgWrQ2INgkh+bbMz68+uJ3Clsh8HSMPEQtAt+BBE6fXDab7KIlsKxU1lIXW/KWVstpdPanJ0pdXpQinDyUQjtY7ZVcfiecRxRDMAUhHFU2cEaciQ+htiPMPx1kdvtWG9T44w3r037ljHBFJdYR0r55qvMRixtAEFJAqA4T1ES87FAx7UozXasytg8MftZYt0rjYgLe6EJ5aWvy2qscBSBQ7yehoJIA3wIIZ9ukfkyBb6qnue5ko8W50rpV4kXqWjI5nbGRXrNW0tBZHXlY48nSgcUXBHWT4GcgLZJoLlKJnV96kCYpq9eWHh7xJzkCAyrQuQ5AJ0qq/uZ3toJglNterev+Qm0KXxPg/+YbFRJdfhbp1wOnVOEYdVHTya6CtO0afhEaBhx3oHwCb5Kq6RwHDzFMl2vfjL8GwzcCoTj7wZe+UFnYDV2yKpPU9dba29gYBdNqJg/KXozO+CJTlKmlKhnqTf5doeS35DZFV+cYJQVjd+oVY/Gtc/6XPzUxb1gMqf6cEjNNoRC8AObrp+fx0cVtGu4ffC2TgXRC8zPl8moUHCB5HZ25d87mlsiiK0aNwBtcEQjRNBT/QrXbw/8aVXdKMHn9EqYEKEyxSGTpYQOaes1G1Qq8pDgqkZtlO2HRyCXpmeM7TSrRPkAh004BfisVpF6zP44n2Jvxz/gOVocNCyy9V6lkod28QM4pbaMvVJigD/w3BrsjSJrXlqc4ulBYOCceiBN4b/gHajYyupbhEt63a619Ay4wsL6a6w6B+A7TnoyE7BliWHJfzVxxIKM/W3M/J8Bx99Op863Q8eNuIMGRx++VbYfjm+VGYBA3Ap/KEu/wxBNBpJJncwHPG45V8Gh98ZIrGCc20MwijGowZbcS7d1nEgcOW5cddZpHL2XPAIRbColiheZzXTvBxZOY3iMSDSKDrICyJ/iQs1vdplVdH/JrLJsQ2jtTnfCrITIghq3KFX3qAgLWAIp8IffNSdTYptnbGfc8s+qcr3zyzyHp1aJg+jxTF4kD1ry5Wauv5V3xnOGwTFecNzXSLHBW20/pCQjk4uorD0plIhMSTc79+/r4RKPClRYTBYex1Ob5crtfvRQBBv6re/6FhtCqtduag67glqRA77/3ulblh9YRtMdDxkCyJDeNnAuCLPQFmdRRWJtH20Z8DstfJf+5oj5SSB64d0iF5/Ya4KfTWxfivj9Ap2/zbYaTo/1gO3tM6RYsCZharMBFr7Fm61mLSrQnEI4OF1gbVS4k/JE9UotOrnLJZuswoWodCSV8zbybkJSVIP7n8UaE9xCR39rJZmf27HOAPVOGc9pdkQUcRrI0qyVF9Z3j1RHDbxIfwbWzmPVjwIdPJvtmBYwEQIUsIW1S939hcVikK00ozPRI02cqhzVUNzpOxVdrwRPvlh1aIOf0xFEqD3YkGnCnFah/cFN3J2gB7N+bZSGawwkKFu1tpQMrp1W+27YNkyT0TpcFpTqgOqqLabrgcCUPxh97mREOGy4xItzQ9xSl6rq+8BZsHcrQFReS+QeMxJ3P6CnL9EP/eOLDjumLhvrcQrpPiknsofbzBv9gTP0lU+TIVwE6E7CcKfT36q+ZiEOHJ9ayf0dyUJLezAb2M8aNHwd0+OJmsVgTzRWA", Bo = 44032, Vo = 4352, Ho = 4449, Uo = 4519, Wo = 28, Go = 588, Ko = 55204, qo = 4371, Jo = 4470, Yo = 4547;
function Xo(e) {
	return e >> 24 & 255;
}
function Zo(e) {
	return e & 16777215;
}
var Qo, $o, es, ts;
function ns() {
	let e = So(zo);
	Qo = new Map(Do(e).flatMap((e, t) => e.map((e) => [e, t + 1 << 24]))), $o = new Set(Eo(e)), es = /* @__PURE__ */ new Map(), ts = /* @__PURE__ */ new Map();
	for (let [t, n] of Oo(e)) {
		if (!$o.has(t) && n.length == 2) {
			let [e, r] = n, i = ts.get(e);
			i || (i = /* @__PURE__ */ new Map(), ts.set(e, i)), i.set(r, t);
		}
		es.set(t, n.reverse());
	}
}
function rs(e) {
	return e >= Bo && e < Ko;
}
function is(e, t) {
	if (e >= Vo && e < qo && t >= Ho && t < Jo) return Bo + (e - Vo) * Go + (t - Ho) * Wo;
	if (rs(e) && t > Uo && t < Yo && (e - Bo) % Wo == 0) return e + (t - Uo);
	{
		let n = ts.get(e);
		return n && (n = n.get(t), n) ? n : -1;
	}
}
function as(e) {
	Qo || ns();
	let t = [], n = [], r = !1;
	function i(e) {
		let n = Qo.get(e);
		n && (r = !0, e |= n), t.push(e);
	}
	for (let r of e) for (;;) {
		if (r < 128) t.push(r);
		else if (rs(r)) {
			let e = r - Bo, t = e / Go | 0, n = e % Go / Wo | 0, a = e % Wo;
			i(Vo + t), i(Ho + n), a > 0 && i(Uo + a);
		} else {
			let e = es.get(r);
			e ? n.push(...e) : i(r);
		}
		if (!n.length) break;
		r = n.pop();
	}
	if (r && t.length > 1) {
		let e = Xo(t[0]);
		for (let n = 1; n < t.length; n++) {
			let r = Xo(t[n]);
			if (r == 0 || e <= r) {
				e = r;
				continue;
			}
			let i = n - 1;
			for (;;) {
				let n = t[i + 1];
				if (t[i + 1] = t[i], t[i] = n, !i || (e = Xo(t[--i]), e <= r)) break;
			}
			e = Xo(t[n]);
		}
	}
	return t;
}
function os(e) {
	let t = [], n = [], r = -1, i = 0;
	for (let a of e) {
		let e = Xo(a), o = Zo(a);
		if (r == -1) e == 0 ? r = o : t.push(o);
		else if (i > 0 && i >= e) e == 0 ? (t.push(r, ...n), n.length = 0, r = o) : n.push(o), i = e;
		else {
			let a = is(r, o);
			a >= 0 ? r = a : i == 0 && e == 0 ? (t.push(r), r = o) : (n.push(o), i = e);
		}
	}
	return r >= 0 && t.push(r, ...n), t;
}
function ss(e) {
	return as(e).map(Zo);
}
function cs(e) {
	return os(as(e));
}
var ls = 45, us = ".", ds = 65039, fs = 1, ps = (e) => Array.from(e);
function ms(e, t) {
	return e.P.has(t) || e.Q.has(t);
}
var hs = class extends Array {
	get is_emoji() {
		return !0;
	}
}, gs, _s, vs, ys, bs, xs, Ss, Cs, ws, Ts, Es;
function Ds() {
	if (gs) return;
	let e = So(_o), t = () => Eo(e), n = () => new Set(t()), r = (e, t) => t.forEach((t) => e.add(t));
	gs = new Map(Oo(e)), _s = n(), vs = t(), ys = new Set(t().map((e) => vs[e])), vs = new Set(vs), bs = n(), n();
	let i = Do(e), a = e(), o = () => {
		let e = /* @__PURE__ */ new Set();
		return t().forEach((t) => r(e, i[t])), r(e, t()), e;
	};
	xs = ko((t) => {
		let n = ko(e).map((e) => e + 96);
		if (n.length) {
			let r = t >= a;
			n[0] -= 32, n = Lo(n), r && (n = `Restricted[${n}]`);
			let i = o(), s = o(), c = !e();
			return {
				N: n,
				P: i,
				Q: s,
				M: c,
				R: r
			};
		}
	}), Ss = n(), Cs = /* @__PURE__ */ new Map();
	let s = t().concat(ps(Ss)).sort((e, t) => e - t);
	s.forEach((t, n) => {
		let r = e(), i = s[n] = r ? s[n - r] : {
			V: [],
			M: /* @__PURE__ */ new Map()
		};
		i.V.push(t), Ss.has(t) || Cs.set(t, i);
	});
	for (let { V: e, M: t } of new Set(Cs.values())) {
		let n = [];
		for (let t of e) {
			let e = xs.filter((e) => ms(e, t)), i = n.find(({ G: t }) => e.some((e) => t.has(e)));
			i || (i = {
				G: /* @__PURE__ */ new Set(),
				V: []
			}, n.push(i)), i.V.push(t), r(i.G, e);
		}
		let i = n.flatMap((e) => ps(e.G));
		for (let { G: e, V: r } of n) {
			let n = new Set(i.filter((t) => !e.has(t)));
			for (let e of r) t.set(e, n);
		}
	}
	ws = /* @__PURE__ */ new Set();
	let c = /* @__PURE__ */ new Set(), l = (e) => ws.has(e) ? c.add(e) : ws.add(e);
	for (let e of xs) {
		for (let t of e.P) l(t);
		for (let t of e.Q) l(t);
	}
	for (let e of ws) !Cs.has(e) && !c.has(e) && Cs.set(e, fs);
	r(ws, ss(ws)), Ts = No(e).map((e) => hs.from(e)).sort(Ro), Es = /* @__PURE__ */ new Map();
	for (let e of Ts) {
		let t = [Es];
		for (let n of e) {
			let e = t.map((e) => {
				let t = e.get(n);
				return t || (t = /* @__PURE__ */ new Map(), e.set(n, t)), t;
			});
			n === ds ? t.push(...e) : t = e;
		}
		for (let n of t) n.V = e;
	}
}
function Os(e) {
	return (Fs(e) ? "" : `${ks(Ns([e]))} `) + Fo(e);
}
function ks(e) {
	return `"${e}"\u200E`;
}
function As(e) {
	if (e.length >= 4 && e[2] == ls && e[3] == ls) throw Error(`invalid label extension: "${Lo(e.slice(0, 4))}"`);
}
function js(e) {
	for (let t = e.lastIndexOf(95); t > 0;) if (e[--t] !== 95) throw Error("underscore allowed only at start");
}
function Ms(e) {
	let t = e[0], n = vo.get(t);
	if (n) throw Us(`leading ${n}`);
	let r = e.length, i = -1;
	for (let a = 1; a < r; a++) {
		t = e[a];
		let r = vo.get(t);
		if (r) {
			if (i == a) throw Us(`${n} + ${r}`);
			i = a + 1, n = r;
		}
	}
	if (i == r) throw Us(`trailing ${n}`);
}
function Ns(e, t = Infinity, n = Fo) {
	let r = [];
	Ps(e[0]) && r.push("◌"), e.length > t && (t >>= 1, e = [
		...e.slice(0, t),
		8230,
		...e.slice(-t)
	]);
	let i = 0, a = e.length;
	for (let t = 0; t < a; t++) {
		let a = e[t];
		Fs(a) && (r.push(Lo(e.slice(i, t))), r.push(n(a)), i = t + 1);
	}
	return r.push(Lo(e.slice(i, a))), r.join("");
}
function Ps(e, t) {
	return Ds(), t ? ys.has(e) : vs.has(e);
}
function Fs(e) {
	return Ds(), bs.has(e);
}
function Is(e) {
	return Bs(Ls(e, cs, Ks));
}
function Ls(e, t, n) {
	if (!e) return [];
	Ds();
	let r = 0;
	return e.split(us).map((e) => {
		let i = Io(e), a = {
			input: i,
			offset: r
		};
		r += i.length + 1;
		try {
			let e = a.tokens = Gs(i, t, n), r = e.length, o;
			if (!r) throw Error("empty label");
			let s = a.output = e.flat();
			if (js(s), !(a.emoji = r > 1 || e[0].is_emoji) && s.every((e) => e < 128)) As(s), o = "ASCII";
			else {
				let t = e.flatMap((e) => e.is_emoji ? [] : e);
				if (!t.length) o = "Emoji";
				else {
					if (vs.has(s[0])) throw Us("leading combining mark");
					for (let t = 1; t < r; t++) {
						let n = e[t];
						if (!n.is_emoji && vs.has(n[0])) throw Us(`emoji + combining mark: "${Lo(e[t - 1])} + ${Ns([n[0]])}"`);
					}
					Ms(s);
					let n = ps(new Set(t)), [i] = zs(n);
					Ws(i, t), Rs(i, n), o = i.N;
				}
			}
			a.type = o;
		} catch (e) {
			a.error = e;
		}
		return a;
	});
}
function Rs(e, t) {
	let n, r = [];
	for (let e of t) {
		let t = Cs.get(e);
		if (t === fs) return;
		if (t) {
			let r = t.M.get(e);
			if (n = n ? n.filter((e) => r.has(e)) : ps(r), !n.length) return;
		} else r.push(e);
	}
	if (n) {
		for (let t of n) if (r.every((e) => ms(t, e))) throw Error(`whole-script confusable: ${e.N}/${t.N}`);
	}
}
function zs(e) {
	let t = xs;
	for (let n of e) {
		let e = t.filter((e) => ms(e, n));
		if (!e.length) throw xs.some((e) => ms(e, n)) ? Hs(t[0], n) : Vs(n);
		if (t = e, e.length == 1) break;
	}
	return t;
}
function Bs(e) {
	return e.map(({ input: t, error: n, output: r }) => {
		if (n) {
			let r = n.message;
			throw Error(e.length == 1 ? r : `Invalid label ${ks(Ns(t, 63))}: ${r}`);
		}
		return Lo(r);
	}).join(us);
}
function Vs(e) {
	return /* @__PURE__ */ Error(`disallowed character: ${Os(e)}`);
}
function Hs(e, t) {
	let n = Os(t), r = xs.find((e) => e.P.has(t));
	return r && (n = `${r.N} ${n}`), /* @__PURE__ */ Error(`illegal mixture: ${e.N} + ${n}`);
}
function Us(e) {
	return /* @__PURE__ */ Error(`illegal placement: ${e}`);
}
function Ws(e, t) {
	for (let n of t) if (!ms(e, n)) throw Hs(e, n);
	if (e.M) {
		let e = ss(t);
		for (let t = 1, n = e.length; t < n; t++) if (ys.has(e[t])) {
			let r = t + 1;
			for (let i; r < n && ys.has(i = e[r]); r++) for (let n = t; n < r; n++) if (e[n] == i) throw Error(`duplicate non-spacing marks: ${Os(i)}`);
			if (r - t > yo) throw Error(`excessive non-spacing marks: ${ks(Ns(e.slice(t - 1, r)))} (${r - t}/${yo})`);
			t = r;
		}
	}
}
function Gs(e, t, n) {
	let r = [], i = [];
	for (e = e.slice().reverse(); e.length;) {
		let a = qs(e);
		if (a) i.length && (r.push(t(i)), i = []), r.push(n(a));
		else {
			let t = e.pop();
			if (ws.has(t)) i.push(t);
			else {
				let e = gs.get(t);
				if (e) i.push(...e);
				else if (!_s.has(t)) throw Vs(t);
			}
		}
	}
	return i.length && r.push(t(i)), r;
}
function Ks(e) {
	return e.filter((e) => e != ds);
}
function qs(e, t) {
	let n = Es, r, i = e.length;
	for (; i && (n = n.get(e[--i]), n);) {
		let { V: a } = n;
		a && (r = a, t && t.push(...e.slice(i).reverse()), e.length = i);
	}
	return r;
}
//#endregion
//#region node_modules/ethers/lib.esm/hash/namehash.js
var Js = /* @__PURE__ */ new Uint8Array(32);
Js.fill(0);
function Ys(e) {
	return d(e.length !== 0, "invalid ENS name; empty component", "comp", e), e;
}
function Xs(e) {
	let t = H(Zs(e)), n = [];
	if (e.length === 0) return n;
	let r = 0;
	for (let e = 0; e < t.length; e++) t[e] === 46 && (n.push(Ys(t.slice(r, e))), r = e + 1);
	return d(r < t.length, "invalid ENS name; empty component", "name", e), n.push(Ys(t.slice(r))), n;
}
function Zs(e) {
	try {
		if (e.length === 0) throw Error("empty label");
		return Is(e);
	} catch (t) {
		d(!1, `invalid ENS name (${t.message})`, "name", e);
	}
}
function Qs(e) {
	try {
		return Xs(e).length !== 0;
	} catch {}
	return !1;
}
function $s(e) {
	d(typeof e == "string", "invalid ENS name; not a string", "name", e), d(e.length, "invalid ENS name (empty label)", "name", e);
	let t = Js, n = Xs(e);
	for (; n.length;) t = W(C([t, W(n.pop())]));
	return S(t);
}
function ec(e, t) {
	let n = t ?? 63;
	return d(n <= 255, "DNS encoded label cannot exceed 255", "length", n), S(C(Xs(e).map((t) => {
		d(t.length <= n, `label ${JSON.stringify(e)} exceeds ${n} bytes`, "name", e);
		let r = new Uint8Array(t.length + 1);
		return r.set(t, 1), r[0] = r.length - 1, r;
	}))) + "00";
}
//#endregion
//#region node_modules/ethers/lib.esm/hash/typed-data.js
var tc = /* @__PURE__ */ new Uint8Array(32);
tc.fill(0);
var nc = BigInt(-1), rc = BigInt(0), ic = BigInt(1), ac = BigInt("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff");
function oc(e) {
	let t = _(e), n = t.length % 32;
	return n ? C([t, tc.slice(n)]) : S(t);
}
var sc = te(ic, 32), cc = te(rc, 32), lc = {
	name: "string",
	version: "string",
	chainId: "uint256",
	verifyingContract: "address",
	salt: "bytes32"
}, uc = [
	"name",
	"version",
	"chainId",
	"verifyingContract",
	"salt"
];
function dc(e) {
	return function(t) {
		return d(typeof t == "string", `invalid domain value for ${JSON.stringify(e)}`, `domain.${e}`, t), t;
	};
}
var fc = {
	name: dc("name"),
	version: dc("version"),
	chainId: function(e) {
		let t = F(e, "domain.chainId");
		return d(t >= 0, "invalid chain ID", "domain.chainId", e), Number.isSafeInteger(t) ? Number(t) : V(t);
	},
	verifyingContract: function(e) {
		try {
			return K(e).toLowerCase();
		} catch {}
		d(!1, "invalid domain value \"verifyingContract\"", "domain.verifyingContract", e);
	},
	salt: function(e) {
		let t = _(e, "domain.salt");
		return d(t.length === 32, "invalid domain value \"salt\"", "domain.salt", e), S(t);
	}
};
function pc(e) {
	{
		let t = e.match(/^(u?)int(\d+)$/);
		if (t) {
			let n = t[1] === "", r = parseInt(t[2]);
			d(r % 8 == 0 && r !== 0 && r <= 256 && t[2] === String(r), "invalid numeric width", "type", e);
			let i = P(ac, n ? r - 1 : r), a = n ? (i + ic) * nc : rc;
			return function(t) {
				let r = F(t, "value");
				return d(r >= a && r <= i, `value out-of-bounds for ${e}`, "value", r), te(n ? N(r, 256) : r, 32);
			};
		}
	}
	{
		let t = e.match(/^bytes(\d+)$/);
		if (t) {
			let n = parseInt(t[1]);
			return d(n !== 0 && n <= 32 && t[1] === String(n), "invalid bytes width", "type", e), function(t) {
				return d(_(t).length === n, `invalid length for ${e}`, "value", t), oc(t);
			};
		}
	}
	switch (e) {
		case "address": return function(e) {
			return D(K(e), 32);
		};
		case "bool": return function(e) {
			return e ? sc : cc;
		};
		case "bytes": return function(e) {
			return W(e);
		};
		case "string": return function(e) {
			return go(e);
		};
	}
	return null;
}
function mc(e, t) {
	return `${e}(${t.map(({ name: e, type: t }) => t + " " + e).join(",")})`;
}
function hc(e) {
	let t = e.match(/^([^\x5b]*)((\x5b\d*\x5d)*)(\x5b(\d*)\x5d)$/);
	return t ? {
		base: t[1],
		index: t[2] + t[4],
		array: {
			base: t[1],
			prefix: t[1] + t[2],
			count: t[5] ? parseInt(t[5]) : -1
		}
	} : { base: e };
}
var gc = class e {
	primaryType;
	#e;
	get types() {
		return JSON.parse(this.#e);
	}
	#t;
	#n;
	constructor(e) {
		this.#t = /* @__PURE__ */ new Map(), this.#n = /* @__PURE__ */ new Map();
		let t = /* @__PURE__ */ new Map(), n = /* @__PURE__ */ new Map(), r = /* @__PURE__ */ new Map(), i = {};
		Object.keys(e).forEach((a) => {
			i[a] = e[a].map(({ name: t, type: n }) => {
				let { base: r, index: i } = hc(n);
				return r === "int" && !e.int && (r = "int256"), r === "uint" && !e.uint && (r = "uint256"), {
					name: t,
					type: r + (i || "")
				};
			}), t.set(a, /* @__PURE__ */ new Set()), n.set(a, []), r.set(a, /* @__PURE__ */ new Set());
		}), this.#e = JSON.stringify(i);
		for (let r in i) {
			let a = /* @__PURE__ */ new Set();
			for (let o of i[r]) {
				d(!a.has(o.name), `duplicate variable name ${JSON.stringify(o.name)} in ${JSON.stringify(r)}`, "types", e), a.add(o.name);
				let i = hc(o.type).base;
				d(i !== r, `circular type reference to ${JSON.stringify(i)}`, "types", e), !pc(i) && (d(n.has(i), `unknown type ${JSON.stringify(i)}`, "types", e), n.get(i).push(r), t.get(r).add(i));
			}
		}
		let o = Array.from(n.keys()).filter((e) => n.get(e).length === 0);
		d(o.length !== 0, "missing primary type", "types", e), d(o.length === 1, `ambiguous primary types or unused types: ${o.map((e) => JSON.stringify(e)).join(", ")}`, "types", e), a(this, { primaryType: o[0] });
		function s(i, a) {
			d(!a.has(i), `circular type reference to ${JSON.stringify(i)}`, "types", e), a.add(i);
			for (let e of t.get(i)) if (n.has(e)) {
				s(e, a);
				for (let t of a) r.get(t).add(e);
			}
			a.delete(i);
		}
		s(this.primaryType, /* @__PURE__ */ new Set());
		for (let [e, t] of r) {
			let n = Array.from(t);
			n.sort(), this.#t.set(e, mc(e, i[e]) + n.map((e) => mc(e, i[e])).join(""));
		}
	}
	getEncoder(e) {
		let t = this.#n.get(e);
		return t || (t = this.#r(e), this.#n.set(e, t)), t;
	}
	#r(e) {
		{
			let t = pc(e);
			if (t) return t;
		}
		let t = hc(e).array;
		if (t) {
			let e = t.prefix, n = this.getEncoder(e);
			return (r) => {
				d(t.count === -1 || t.count === r.length, `array length mismatch; expected length ${t.count}`, "value", r);
				let i = r.map(n);
				return this.#t.has(e) && (i = i.map(W)), W(C(i));
			};
		}
		let n = this.types[e];
		if (n) {
			let t = go(this.#t.get(e));
			return (e) => {
				let r = n.map(({ name: t, type: n }) => {
					let r = this.getEncoder(n)(e[t]);
					return this.#t.has(n) ? W(r) : r;
				});
				return r.unshift(t), C(r);
			};
		}
		d(!1, `unknown type: ${e}`, "type", e);
	}
	encodeType(e) {
		let t = this.#t.get(e);
		return d(t, `unknown type: ${JSON.stringify(e)}`, "name", e), t;
	}
	encodeData(e, t) {
		return this.getEncoder(e)(t);
	}
	hashStruct(e, t) {
		return W(this.encodeData(e, t));
	}
	encode(e) {
		return this.encodeData(this.primaryType, e);
	}
	hash(e) {
		return this.hashStruct(this.primaryType, e);
	}
	_visit(e, t, n) {
		if (pc(e)) return n(e, t);
		let r = hc(e).array;
		if (r) return d(r.count === -1 || r.count === t.length, `array length mismatch; expected length ${r.count}`, "value", t), t.map((e) => this._visit(r.prefix, e, n));
		let i = this.types[e];
		if (i) return i.reduce((e, { name: r, type: i }) => (e[r] = this._visit(i, t[r], n), e), {});
		d(!1, `unknown type: ${e}`, "type", e);
	}
	visit(e, t) {
		return this._visit(this.primaryType, e, t);
	}
	static from(t) {
		return new e(t);
	}
	static getPrimaryType(t) {
		return e.from(t).primaryType;
	}
	static hashStruct(t, n, r) {
		return e.from(n).hashStruct(t, r);
	}
	static hashDomain(t) {
		let n = [];
		for (let e in t) {
			if (t[e] == null) continue;
			let r = lc[e];
			d(r, `invalid typed-data domain key: ${JSON.stringify(e)}`, "domain", t), n.push({
				name: e,
				type: r
			});
		}
		return n.sort((e, t) => uc.indexOf(e.name) - uc.indexOf(t.name)), e.hashStruct("EIP712Domain", { EIP712Domain: n }, t);
	}
	static encode(t, n, r) {
		return C([
			"0x1901",
			e.hashDomain(t),
			e.from(n).hash(r)
		]);
	}
	static hash(t, n, r) {
		return W(e.encode(t, n, r));
	}
	static async resolveNames(t, n, r, i) {
		t = Object.assign({}, t);
		for (let e in t) t[e] ?? delete t[e];
		let a = {};
		t.verifyingContract && !y(t.verifyingContract, 20) && (a[t.verifyingContract] = "0x");
		let o = e.from(n);
		o.visit(r, (e, t) => (e === "address" && !y(t, 20) && (a[t] = "0x"), t));
		for (let e in a) a[e] = await i(e);
		return t.verifyingContract && a[t.verifyingContract] && (t.verifyingContract = a[t.verifyingContract]), r = o.visit(r, (e, t) => e === "address" && a[t] ? a[t] : t), {
			domain: t,
			value: r
		};
	}
	static getPayload(t, n, r) {
		e.hashDomain(t);
		let i = {}, a = [];
		uc.forEach((e) => {
			let n = t[e];
			n != null && (i[e] = fc[e](n), a.push({
				name: e,
				type: lc[e]
			}));
		});
		let o = e.from(n);
		n = o.types;
		let s = Object.assign({}, n);
		return d(s.EIP712Domain == null, "types must not contain EIP712Domain type", "types.EIP712Domain", n), s.EIP712Domain = a, o.encode(r), {
			types: s,
			domain: i,
			primaryType: o.primaryType,
			message: o.visit(r, (e, t) => {
				if (e.match(/^bytes(\d*)/)) return S(_(t));
				if (e.match(/^u?int/)) return F(t).toString();
				switch (e) {
					case "address": return t.toLowerCase();
					case "bool": return !!t;
					case "string": return d(typeof t == "string", "invalid string", "value", t), t;
				}
				d(!1, "unsupported type", "type", e);
			})
		};
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/abi/fragments.js
function _c(e) {
	let t = /* @__PURE__ */ new Set();
	return e.forEach((e) => t.add(e)), Object.freeze(t);
}
var vc = _c("external public payable override".split(" ")), yc = "constant external internal payable private public pure view override", bc = _c(yc.split(" ")), xc = "constructor error event fallback function receive struct", Sc = _c(xc.split(" ")), Cc = "calldata memory storage payable indexed", wc = _c(Cc.split(" ")), Tc = _c([
	xc,
	Cc,
	"tuple returns",
	yc
].join(" ").split(" ")), Ec = {
	"(": "OPEN_PAREN",
	")": "CLOSE_PAREN",
	"[": "OPEN_BRACKET",
	"]": "CLOSE_BRACKET",
	",": "COMMA",
	"@": "AT"
}, Dc = /* @__PURE__ */ RegExp("^(\\s*)"), Oc = /* @__PURE__ */ RegExp("^([0-9]+)"), kc = /* @__PURE__ */ RegExp("^([a-zA-Z$_][a-zA-Z0-9$_]*)"), Ac = /* @__PURE__ */ RegExp("^([a-zA-Z$_][a-zA-Z0-9$_]*)$"), jc = /* @__PURE__ */ RegExp("^(address|bool|bytes([0-9]*)|string|u?int([0-9]*))$"), Mc = class e {
	#e;
	#t;
	get offset() {
		return this.#e;
	}
	get length() {
		return this.#t.length - this.#e;
	}
	constructor(e) {
		this.#e = 0, this.#t = e.slice();
	}
	clone() {
		return new e(this.#t);
	}
	reset() {
		this.#e = 0;
	}
	#n(t = 0, n = 0) {
		return new e(this.#t.slice(t, n).map((e) => Object.freeze(Object.assign({}, e, {
			match: e.match - t,
			linkBack: e.linkBack - t,
			linkNext: e.linkNext - t
		}))));
	}
	popKeyword(e) {
		let t = this.peek();
		if (t.type !== "KEYWORD" || !e.has(t.text)) throw Error(`expected keyword ${t.text}`);
		return this.pop().text;
	}
	popType(e) {
		if (this.peek().type !== e) {
			let t = this.peek();
			throw Error(`expected ${e}; got ${t.type} ${JSON.stringify(t.text)}`);
		}
		return this.pop().text;
	}
	popParen() {
		let e = this.peek();
		if (e.type !== "OPEN_PAREN") throw Error("bad start");
		let t = this.#n(this.#e + 1, e.match + 1);
		return this.#e = e.match + 1, t;
	}
	popParams() {
		let e = this.peek();
		if (e.type !== "OPEN_PAREN") throw Error("bad start");
		let t = [];
		for (; this.#e < e.match - 1;) {
			let e = this.peek().linkNext;
			t.push(this.#n(this.#e + 1, e)), this.#e = e;
		}
		return this.#e = e.match + 1, t;
	}
	peek() {
		if (this.#e >= this.#t.length) throw Error("out-of-bounds");
		return this.#t[this.#e];
	}
	peekKeyword(e) {
		let t = this.peekType("KEYWORD");
		return t != null && e.has(t) ? t : null;
	}
	peekType(e) {
		if (this.length === 0) return null;
		let t = this.peek();
		return t.type === e ? t.text : null;
	}
	pop() {
		let e = this.peek();
		return this.#e++, e;
	}
	toString() {
		let e = [];
		for (let t = this.#e; t < this.#t.length; t++) {
			let n = this.#t[t];
			e.push(`${n.type}:${n.text}`);
		}
		return `<TokenString ${e.join(" ")}>`;
	}
};
function Nc(e) {
	let t = [], n = (t) => {
		let n = a < e.length ? JSON.stringify(e[a]) : "$EOI";
		throw Error(`invalid token ${n} at ${a}: ${t}`);
	}, r = [], i = [], a = 0;
	for (; a < e.length;) {
		let o = e.substring(a), s = o.match(Dc);
		s && (a += s[1].length, o = e.substring(a));
		let c = {
			depth: r.length,
			linkBack: -1,
			linkNext: -1,
			match: -1,
			type: "",
			text: "",
			offset: a,
			value: -1
		};
		t.push(c);
		let l = Ec[o[0]] || "";
		if (l) {
			if (c.type = l, c.text = o[0], a++, l === "OPEN_PAREN") r.push(t.length - 1), i.push(t.length - 1);
			else if (l == "CLOSE_PAREN") r.length === 0 && n("no matching open bracket"), c.match = r.pop(), t[c.match].match = t.length - 1, c.depth--, c.linkBack = i.pop(), t[c.linkBack].linkNext = t.length - 1;
			else if (l === "COMMA") c.linkBack = i.pop(), t[c.linkBack].linkNext = t.length - 1, i.push(t.length - 1);
			else if (l === "OPEN_BRACKET") c.type = "BRACKET";
			else if (l === "CLOSE_BRACKET") {
				let e = t.pop().text;
				if (t.length > 0 && t[t.length - 1].type === "NUMBER") {
					let n = t.pop().text;
					e = n + e, t[t.length - 1].value = z(n);
				}
				if (t.length === 0 || t[t.length - 1].type !== "BRACKET") throw Error("missing opening bracket");
				t[t.length - 1].text += e;
			}
			continue;
		}
		if (s = o.match(kc), s) {
			if (c.text = s[1], a += c.text.length, Tc.has(c.text)) {
				c.type = "KEYWORD";
				continue;
			}
			if (c.text.match(jc)) {
				c.type = "TYPE";
				continue;
			}
			c.type = "ID";
			continue;
		}
		if (s = o.match(Oc), s) {
			c.text = s[1], c.type = "NUMBER", a += c.text.length;
			continue;
		}
		throw Error(`unexpected token ${JSON.stringify(o[0])} at position ${a}`);
	}
	return new Mc(t.map((e) => Object.freeze(e)));
}
function Pc(e, t) {
	let n = [];
	for (let r in t.keys()) e.has(r) && n.push(r);
	if (n.length > 1) throw Error(`conflicting types: ${n.join(", ")}`);
}
function Fc(e, t) {
	if (t.peekKeyword(Sc)) {
		let n = t.pop().text;
		if (n !== e) throw Error(`expected ${e}, got ${n}`);
	}
	return t.popType("ID");
}
function Ic(e, t) {
	let n = /* @__PURE__ */ new Set();
	for (;;) {
		let r = e.peekType("KEYWORD");
		if (r == null || t && !t.has(r)) break;
		if (e.pop(), n.has(r)) throw Error(`duplicate keywords: ${JSON.stringify(r)}`);
		n.add(r);
	}
	return Object.freeze(n);
}
function Lc(e) {
	let t = Ic(e, bc);
	return Pc(t, _c("constant payable nonpayable".split(" "))), Pc(t, _c("pure view payable nonpayable".split(" "))), t.has("view") ? "view" : t.has("pure") ? "pure" : t.has("payable") ? "payable" : t.has("nonpayable") ? "nonpayable" : t.has("constant") ? "view" : "nonpayable";
}
function Rc(e, t) {
	return e.popParams().map((e) => Qc.from(e, t));
}
function zc(e) {
	if (e.peekType("AT")) {
		if (e.pop(), e.peekType("NUMBER")) return F(e.pop().text);
		throw Error("invalid gas");
	}
	return null;
}
function Bc(e) {
	if (e.length) throw Error(`unexpected tokens at offset ${e.offset}: ${e.toString()}`);
}
var Vc = /* @__PURE__ */ new RegExp(/^(.*)\[([0-9]*)\]$/);
function Hc(e) {
	let t = e.match(jc);
	if (d(t, "invalid type", "type", e), e === "uint") return "uint256";
	if (e === "int") return "int256";
	if (t[2]) {
		let n = parseInt(t[2]);
		d(n !== 0 && n <= 32, "invalid bytes length", "type", e);
	} else if (t[3]) {
		let n = parseInt(t[3]);
		d(n !== 0 && n <= 256 && n % 8 == 0, "invalid numeric width", "type", e);
	}
	return e;
}
var Uc = {}, Wc = Symbol.for("_ethers_internal"), Gc = "_ParamTypeInternal", Kc = "_ErrorInternal", qc = "_EventInternal", Jc = "_ConstructorInternal", Yc = "_FallbackInternal", Xc = "_FunctionInternal", Zc = "_StructInternal", Qc = class e {
	name;
	type;
	baseType;
	indexed;
	components;
	arrayLength;
	arrayChildren;
	constructor(e, t, n, r, i, o, s, c) {
		if (h(e, Uc, "ParamType"), Object.defineProperty(this, Wc, { value: Gc }), o &&= Object.freeze(o.slice()), r === "array") {
			if (s == null || c == null) throw Error("");
		} else if (s != null || c != null) throw Error("");
		if (r === "tuple") {
			if (o == null) throw Error("");
		} else if (o != null) throw Error("");
		a(this, {
			name: t,
			type: n,
			baseType: r,
			indexed: i,
			components: o,
			arrayLength: s,
			arrayChildren: c
		});
	}
	format(e) {
		if (e ??= "sighash", e === "json") {
			let t = this.name || "";
			if (this.isArray()) {
				let e = JSON.parse(this.arrayChildren.format("json"));
				return e.name = t, e.type += `[${this.arrayLength < 0 ? "" : String(this.arrayLength)}]`, JSON.stringify(e);
			}
			let n = {
				type: this.baseType === "tuple" ? "tuple" : this.type,
				name: t
			};
			return typeof this.indexed == "boolean" && (n.indexed = this.indexed), this.isTuple() && (n.components = this.components.map((t) => JSON.parse(t.format(e)))), JSON.stringify(n);
		}
		let t = "";
		return this.isArray() ? (t += this.arrayChildren.format(e), t += `[${this.arrayLength < 0 ? "" : String(this.arrayLength)}]`) : this.isTuple() ? t += "(" + this.components.map((t) => t.format(e)).join(e === "full" ? ", " : ",") + ")" : t += this.type, e !== "sighash" && (this.indexed === !0 && (t += " indexed"), e === "full" && this.name && (t += " " + this.name)), t;
	}
	isArray() {
		return this.baseType === "array";
	}
	isTuple() {
		return this.baseType === "tuple";
	}
	isIndexable() {
		return this.indexed != null;
	}
	walk(e, t) {
		if (this.isArray()) {
			if (!Array.isArray(e)) throw Error("invalid array value");
			if (this.arrayLength !== -1 && e.length !== this.arrayLength) throw Error("array is wrong length");
			let n = this;
			return e.map((e) => n.arrayChildren.walk(e, t));
		}
		if (this.isTuple()) {
			if (!Array.isArray(e)) throw Error("invalid tuple value");
			if (e.length !== this.components.length) throw Error("array is wrong length");
			let n = this;
			return e.map((e, r) => n.components[r].walk(e, t));
		}
		return t(this.type, e);
	}
	#e(e, t, n, r) {
		if (this.isArray()) {
			if (!Array.isArray(t)) throw Error("invalid array value");
			if (this.arrayLength !== -1 && t.length !== this.arrayLength) throw Error("array is wrong length");
			let i = this.arrayChildren, a = t.slice();
			a.forEach((t, r) => {
				i.#e(e, t, n, (e) => {
					a[r] = e;
				});
			}), r(a);
			return;
		}
		if (this.isTuple()) {
			let i = this.components, a;
			if (Array.isArray(t)) a = t.slice();
			else {
				if (typeof t != "object" || !t) throw Error("invalid tuple value");
				a = i.map((e) => {
					if (!e.name) throw Error("cannot use object value with unnamed components");
					if (!(e.name in t)) throw Error(`missing value for component ${e.name}`);
					return t[e.name];
				});
			}
			if (a.length !== this.components.length) throw Error("array is wrong length");
			a.forEach((t, r) => {
				i[r].#e(e, t, n, (e) => {
					a[r] = e;
				});
			}), r(a);
			return;
		}
		let i = n(this.type, t);
		i.then ? e.push((async function() {
			r(await i);
		})()) : r(i);
	}
	async walkAsync(e, t) {
		let n = [], r = [e];
		return this.#e(n, e, t, (e) => {
			r[0] = e;
		}), n.length && await Promise.all(n), r[0];
	}
	static from(t, n) {
		if (e.isParamType(t)) return t;
		if (typeof t == "string") try {
			return e.from(Nc(t), n);
		} catch {
			d(!1, "invalid param type", "obj", t);
		}
		else if (t instanceof Mc) {
			let r = "", i = "", a = null;
			Ic(t, _c(["tuple"])).has("tuple") || t.peekType("OPEN_PAREN") ? (i = "tuple", a = t.popParams().map((t) => e.from(t)), r = `tuple(${a.map((e) => e.format()).join(",")})`) : (r = Hc(t.popType("TYPE")), i = r);
			let o = null, s = null;
			for (; t.length && t.peekType("BRACKET");) {
				let n = t.pop();
				o = new e(Uc, "", r, i, null, a, s, o), s = n.value, r += n.text, i = "array", a = null;
			}
			let c = null;
			if (Ic(t, wc).has("indexed")) {
				if (!n) throw Error("");
				c = !0;
			}
			let l = t.peekType("ID") ? t.pop().text : "";
			if (t.length) throw Error("leftover tokens");
			return new e(Uc, l, r, i, c, a, s, o);
		}
		let r = t.name;
		d(!r || typeof r == "string" && r.match(Ac), "invalid name", "obj.name", r);
		let i = t.indexed;
		i != null && (d(n, "parameter cannot be indexed", "obj.indexed", t.indexed), i = !!i);
		let a = t.type, o = a.match(Vc);
		if (o) {
			let n = parseInt(o[2] || "-1"), s = e.from({
				type: o[1],
				components: t.components
			});
			return new e(Uc, r || "", a, "array", i, null, n, s);
		}
		if (a === "tuple" || a.startsWith("tuple(") || a.startsWith("(")) {
			let n = t.components == null ? null : t.components.map((t) => e.from(t));
			return new e(Uc, r || "", a, "tuple", i, n, null, null);
		}
		return a = Hc(t.type), new e(Uc, r || "", a, a, i, null, null, null);
	}
	static isParamType(e) {
		return e && e[Wc] === Gc;
	}
}, $c = class e {
	type;
	inputs;
	constructor(e, t, n) {
		h(e, Uc, "Fragment"), n = Object.freeze(n.slice()), a(this, {
			type: t,
			inputs: n
		});
	}
	static from(t) {
		if (typeof t == "string") {
			try {
				e.from(JSON.parse(t));
			} catch {}
			return e.from(Nc(t));
		}
		if (t instanceof Mc) switch (t.peekKeyword(Sc)) {
			case "constructor": return il.from(t);
			case "error": return nl.from(t);
			case "event": return rl.from(t);
			case "fallback":
			case "receive": return al.from(t);
			case "function": return ol.from(t);
			case "struct": return sl.from(t);
		}
		else if (typeof t == "object") {
			switch (t.type) {
				case "constructor": return il.from(t);
				case "error": return nl.from(t);
				case "event": return rl.from(t);
				case "fallback":
				case "receive": return al.from(t);
				case "function": return ol.from(t);
				case "struct": return sl.from(t);
			}
			u(!1, `unsupported type: ${t.type}`, "UNSUPPORTED_OPERATION", { operation: "Fragment.from" });
		}
		d(!1, "unsupported frgament object", "obj", t);
	}
	static isConstructor(e) {
		return il.isFragment(e);
	}
	static isError(e) {
		return nl.isFragment(e);
	}
	static isEvent(e) {
		return rl.isFragment(e);
	}
	static isFunction(e) {
		return ol.isFragment(e);
	}
	static isStruct(e) {
		return sl.isFragment(e);
	}
}, el = class extends $c {
	name;
	constructor(e, t, n, r) {
		super(e, t, r), d(typeof n == "string" && n.match(Ac), "invalid identifier", "name", n), r = Object.freeze(r.slice()), a(this, { name: n });
	}
};
function tl(e, t) {
	return "(" + t.map((t) => t.format(e)).join(e === "full" ? ", " : ",") + ")";
}
var nl = class e extends el {
	constructor(e, t, n) {
		super(e, "error", t, n), Object.defineProperty(this, Wc, { value: Kc });
	}
	get selector() {
		return go(this.format("sighash")).substring(0, 10);
	}
	format(e) {
		if (e ??= "sighash", e === "json") return JSON.stringify({
			type: "error",
			name: this.name,
			inputs: this.inputs.map((t) => JSON.parse(t.format(e)))
		});
		let t = [];
		return e !== "sighash" && t.push("error"), t.push(this.name + tl(e, this.inputs)), t.join(" ");
	}
	static from(t) {
		if (e.isFragment(t)) return t;
		if (typeof t == "string") return e.from(Nc(t));
		if (t instanceof Mc) {
			let n = Fc("error", t), r = Rc(t);
			return Bc(t), new e(Uc, n, r);
		}
		return new e(Uc, t.name, t.inputs ? t.inputs.map(Qc.from) : []);
	}
	static isFragment(e) {
		return e && e[Wc] === Kc;
	}
}, rl = class e extends el {
	anonymous;
	constructor(e, t, n, r) {
		super(e, "event", t, n), Object.defineProperty(this, Wc, { value: qc }), a(this, { anonymous: r });
	}
	get topicHash() {
		return go(this.format("sighash"));
	}
	format(e) {
		if (e ??= "sighash", e === "json") return JSON.stringify({
			type: "event",
			anonymous: this.anonymous,
			name: this.name,
			inputs: this.inputs.map((t) => JSON.parse(t.format(e)))
		});
		let t = [];
		return e !== "sighash" && t.push("event"), t.push(this.name + tl(e, this.inputs)), e !== "sighash" && this.anonymous && t.push("anonymous"), t.join(" ");
	}
	static getTopicHash(t, n) {
		return n = (n || []).map((e) => Qc.from(e)), new e(Uc, t, n, !1).topicHash;
	}
	static from(t) {
		if (e.isFragment(t)) return t;
		if (typeof t == "string") try {
			return e.from(Nc(t));
		} catch {
			d(!1, "invalid event fragment", "obj", t);
		}
		else if (t instanceof Mc) {
			let n = Fc("event", t), r = Rc(t, !0), i = !!Ic(t, _c(["anonymous"])).has("anonymous");
			return Bc(t), new e(Uc, n, r, i);
		}
		return new e(Uc, t.name, t.inputs ? t.inputs.map((e) => Qc.from(e, !0)) : [], !!t.anonymous);
	}
	static isFragment(e) {
		return e && e[Wc] === qc;
	}
}, il = class e extends $c {
	payable;
	gas;
	constructor(e, t, n, r, i) {
		super(e, t, n), Object.defineProperty(this, Wc, { value: Jc }), a(this, {
			payable: r,
			gas: i
		});
	}
	format(e) {
		if (u(e != null && e !== "sighash", "cannot format a constructor for sighash", "UNSUPPORTED_OPERATION", { operation: "format(sighash)" }), e === "json") return JSON.stringify({
			type: "constructor",
			stateMutability: this.payable ? "payable" : "undefined",
			payable: this.payable,
			gas: this.gas == null ? void 0 : this.gas,
			inputs: this.inputs.map((t) => JSON.parse(t.format(e)))
		});
		let t = [`constructor${tl(e, this.inputs)}`];
		return this.payable && t.push("payable"), this.gas != null && t.push(`@${this.gas.toString()}`), t.join(" ");
	}
	static from(t) {
		if (e.isFragment(t)) return t;
		if (typeof t == "string") try {
			return e.from(Nc(t));
		} catch {
			d(!1, "invalid constuctor fragment", "obj", t);
		}
		else if (t instanceof Mc) {
			Ic(t, _c(["constructor"]));
			let n = Rc(t), r = !!Ic(t, vc).has("payable"), i = zc(t);
			return Bc(t), new e(Uc, "constructor", n, r, i);
		}
		return new e(Uc, "constructor", t.inputs ? t.inputs.map(Qc.from) : [], !!t.payable, t.gas == null ? null : t.gas);
	}
	static isFragment(e) {
		return e && e[Wc] === Jc;
	}
}, al = class e extends $c {
	payable;
	constructor(e, t, n) {
		super(e, "fallback", t), Object.defineProperty(this, Wc, { value: Yc }), a(this, { payable: n });
	}
	format(e) {
		let t = this.inputs.length === 0 ? "receive" : "fallback";
		if (e === "json") {
			let e = this.payable ? "payable" : "nonpayable";
			return JSON.stringify({
				type: t,
				stateMutability: e
			});
		}
		return `${t}()${this.payable ? " payable" : ""}`;
	}
	static from(t) {
		if (e.isFragment(t)) return t;
		if (typeof t == "string") try {
			return e.from(Nc(t));
		} catch {
			d(!1, "invalid fallback fragment", "obj", t);
		}
		else if (t instanceof Mc) {
			let n = t.toString();
			if (d(t.peekKeyword(_c(["fallback", "receive"])), "type must be fallback or receive", "obj", n), t.popKeyword(_c(["fallback", "receive"])) === "receive") {
				let n = Rc(t);
				return d(n.length === 0, "receive cannot have arguments", "obj.inputs", n), Ic(t, _c(["payable"])), Bc(t), new e(Uc, [], !0);
			}
			let r = Rc(t);
			r.length ? d(r.length === 1 && r[0].type === "bytes", "invalid fallback inputs", "obj.inputs", r.map((e) => e.format("minimal")).join(", ")) : r = [Qc.from("bytes")];
			let i = Lc(t);
			if (d(i === "nonpayable" || i === "payable", "fallback cannot be constants", "obj.stateMutability", i), Ic(t, _c(["returns"])).has("returns")) {
				let e = Rc(t);
				d(e.length === 1 && e[0].type === "bytes", "invalid fallback outputs", "obj.outputs", e.map((e) => e.format("minimal")).join(", "));
			}
			return Bc(t), new e(Uc, r, i === "payable");
		}
		if (t.type === "receive") return new e(Uc, [], !0);
		if (t.type === "fallback") {
			let n = [Qc.from("bytes")], r = t.stateMutability === "payable";
			return new e(Uc, n, r);
		}
		d(!1, "invalid fallback description", "obj", t);
	}
	static isFragment(e) {
		return e && e[Wc] === Yc;
	}
}, ol = class e extends el {
	constant;
	outputs;
	stateMutability;
	payable;
	gas;
	constructor(e, t, n, r, i, o) {
		super(e, "function", t, r), Object.defineProperty(this, Wc, { value: Xc }), i = Object.freeze(i.slice());
		let s = n === "view" || n === "pure", c = n === "payable";
		a(this, {
			constant: s,
			gas: o,
			outputs: i,
			payable: c,
			stateMutability: n
		});
	}
	get selector() {
		return go(this.format("sighash")).substring(0, 10);
	}
	format(e) {
		if (e ??= "sighash", e === "json") return JSON.stringify({
			type: "function",
			name: this.name,
			constant: this.constant,
			stateMutability: this.stateMutability === "nonpayable" ? void 0 : this.stateMutability,
			payable: this.payable,
			gas: this.gas == null ? void 0 : this.gas,
			inputs: this.inputs.map((t) => JSON.parse(t.format(e))),
			outputs: this.outputs.map((t) => JSON.parse(t.format(e)))
		});
		let t = [];
		return e !== "sighash" && t.push("function"), t.push(this.name + tl(e, this.inputs)), e !== "sighash" && (this.stateMutability !== "nonpayable" && t.push(this.stateMutability), this.outputs && this.outputs.length && (t.push("returns"), t.push(tl(e, this.outputs))), this.gas != null && t.push(`@${this.gas.toString()}`)), t.join(" ");
	}
	static getSelector(t, n) {
		return n = (n || []).map((e) => Qc.from(e)), new e(Uc, t, "view", n, [], null).selector;
	}
	static from(t) {
		if (e.isFragment(t)) return t;
		if (typeof t == "string") try {
			return e.from(Nc(t));
		} catch {
			d(!1, "invalid function fragment", "obj", t);
		}
		else if (t instanceof Mc) {
			let n = Fc("function", t), r = Rc(t), i = Lc(t), a = [];
			Ic(t, _c(["returns"])).has("returns") && (a = Rc(t));
			let o = zc(t);
			return Bc(t), new e(Uc, n, i, r, a, o);
		}
		let n = t.stateMutability;
		return n ?? (n = "payable", typeof t.constant == "boolean" ? (n = "view", t.constant || (n = "payable", typeof t.payable == "boolean" && !t.payable && (n = "nonpayable"))) : typeof t.payable == "boolean" && !t.payable && (n = "nonpayable")), new e(Uc, t.name, n, t.inputs ? t.inputs.map(Qc.from) : [], t.outputs ? t.outputs.map(Qc.from) : [], t.gas == null ? null : t.gas);
	}
	static isFragment(e) {
		return e && e[Wc] === Xc;
	}
}, sl = class e extends el {
	constructor(e, t, n) {
		super(e, "struct", t, n), Object.defineProperty(this, Wc, { value: Zc });
	}
	format() {
		throw Error("@TODO");
	}
	static from(t) {
		if (typeof t == "string") try {
			return e.from(Nc(t));
		} catch {
			d(!1, "invalid struct fragment", "obj", t);
		}
		else if (t instanceof Mc) {
			let n = Fc("struct", t), r = Rc(t);
			return Bc(t), new e(Uc, n, r);
		}
		return new e(Uc, t.name, t.inputs ? t.inputs.map(Qc.from) : []);
	}
	static isFragment(e) {
		return e && e[Wc] === Zc;
	}
}, cl = /* @__PURE__ */ new Map();
cl.set(0, "GENERIC_PANIC"), cl.set(1, "ASSERT_FALSE"), cl.set(17, "OVERFLOW"), cl.set(18, "DIVIDE_BY_ZERO"), cl.set(33, "ENUM_RANGE_ERROR"), cl.set(34, "BAD_STORAGE_DATA"), cl.set(49, "STACK_UNDERFLOW"), cl.set(50, "ARRAY_RANGE_ERROR"), cl.set(65, "OUT_OF_MEMORY"), cl.set(81, "UNINITIALIZED_FUNCTION_CALL");
var ll = /* @__PURE__ */ new RegExp(/^bytes([0-9]*)$/), ul = /* @__PURE__ */ new RegExp(/^(u?int)([0-9]*)$/), dl = null, fl = 1024;
function pl(e, t, n, r) {
	let i = "missing revert data", a = null, o = null;
	if (n) {
		i = "execution reverted";
		let e = _(n);
		if (n = S(n), e.length === 0) i += " (no data present; likely require(false) occurred", a = "require(false)";
		else if (e.length % 32 != 4) i += " (could not decode reason; invalid data length)";
		else if (S(e.slice(0, 4)) === "0x08c379a0") try {
			a = r.decode(["string"], e.slice(4))[0], o = {
				signature: "Error(string)",
				name: "Error",
				args: [a]
			}, i += `: ${JSON.stringify(a)}`;
		} catch {
			i += " (could not decode reason; invalid string data)";
		}
		else if (S(e.slice(0, 4)) === "0x4e487b71") try {
			let t = Number(r.decode(["uint256"], e.slice(4))[0]);
			o = {
				signature: "Panic(uint256)",
				name: "Panic",
				args: [t]
			}, a = `Panic due to ${cl.get(t) || "UNKNOWN"}(${t})`, i += `: ${a}`;
		} catch {
			i += " (could not decode panic code)";
		}
		else i += " (unknown custom error)";
	}
	let s = {
		to: t.to ? K(t.to) : null,
		data: t.data || "0x"
	};
	return t.from && (s.from = K(t.from)), l(i, "CALL_EXCEPTION", {
		action: e,
		data: n,
		reason: a,
		transaction: s,
		invocation: null,
		revert: o
	});
}
var ml = class e {
	#e(e) {
		if (e.isArray()) return new xa(this.#e(e.arrayChildren), e.arrayLength, e.name);
		if (e.isTuple()) return new Na(e.components.map((e) => this.#e(e)), e.name);
		switch (e.baseType) {
			case "address": return new _a(e.name);
			case "bool": return new Sa(e.name);
			case "string": return new Ma(e.name);
			case "bytes": return new wa(e.name);
			case "": return new Da(e.name);
		}
		let t = e.type.match(ul);
		if (t) {
			let n = parseInt(t[2] || "256");
			return d(n !== 0 && n <= 256 && n % 8 == 0, "invalid " + t[1] + " bit length", "param", e), new ja(n / 8, t[1] === "int", e.name);
		}
		if (t = e.type.match(ll), t) {
			let n = parseInt(t[1]);
			return d(n !== 0 && n <= 32, "invalid bytes length", "param", e), new Ta(n, e.name);
		}
		d(!1, "invalid type", "type", e.type);
	}
	getDefaultValue(e) {
		return new Na(e.map((e) => this.#e(Qc.from(e))), "_").defaultValue();
	}
	encode(e, t) {
		f(t.length, e.length, "types/values length mismatch");
		let n = new Na(e.map((e) => this.#e(Qc.from(e))), "_"), r = new mt();
		return n.encode(r, t), r.data;
	}
	decode(e, t, n) {
		return new Na(e.map((e) => this.#e(Qc.from(e))), "_").decode(new ht(t, n, fl));
	}
	static _setDefaultMaxInflation(e) {
		d(typeof e == "number" && Number.isInteger(e), "invalid defaultMaxInflation factor", "value", e), fl = e;
	}
	static defaultAbiCoder() {
		return dl ??= new e(), dl;
	}
	static getBuiltinCallException(t, n, r) {
		return pl(t, n, r, e.defaultAbiCoder());
	}
}, hl = class {
	fragment;
	name;
	signature;
	topic;
	args;
	constructor(e, t, n) {
		let r = e.name, i = e.format();
		a(this, {
			fragment: e,
			name: r,
			signature: i,
			topic: t,
			args: n
		});
	}
}, gl = class {
	fragment;
	name;
	args;
	signature;
	selector;
	value;
	constructor(e, t, n, r) {
		let i = e.name, o = e.format();
		a(this, {
			fragment: e,
			name: i,
			args: n,
			signature: o,
			selector: t,
			value: r
		});
	}
}, _l = class {
	fragment;
	name;
	args;
	signature;
	selector;
	constructor(e, t, n) {
		let r = e.name, i = e.format();
		a(this, {
			fragment: e,
			name: r,
			args: n,
			signature: i,
			selector: t
		});
	}
}, vl = class {
	hash;
	_isIndexed;
	static isIndexed(e) {
		return !!(e && e._isIndexed);
	}
	constructor(e) {
		a(this, {
			hash: e,
			_isIndexed: !0
		});
	}
}, yl = {
	0: "generic panic",
	1: "assert(false)",
	17: "arithmetic overflow",
	18: "division or modulo by zero",
	33: "enum overflow",
	34: "invalid encoded storage byte array accessed",
	49: "out-of-bounds array access; popping on an empty array",
	50: "out-of-bounds access of an array or bytesN",
	65: "out of memory",
	81: "uninitialized function"
}, bl = {
	"0x08c379a0": {
		signature: "Error(string)",
		name: "Error",
		inputs: ["string"],
		reason: (e) => `reverted with reason string ${JSON.stringify(e)}`
	},
	"0x4e487b71": {
		signature: "Panic(uint256)",
		name: "Panic",
		inputs: ["uint256"],
		reason: (e) => {
			let t = "unknown panic code";
			return e >= 0 && e <= 255 && yl[e.toString()] && (t = yl[e.toString()]), `reverted with panic code 0x${e.toString(16)} (${t})`;
		}
	}
}, xl = class e {
	fragments;
	deploy;
	fallback;
	receive;
	#e;
	#t;
	#n;
	#r;
	constructor(e) {
		let t = [];
		t = typeof e == "string" ? JSON.parse(e) : e, this.#n = /* @__PURE__ */ new Map(), this.#e = /* @__PURE__ */ new Map(), this.#t = /* @__PURE__ */ new Map();
		let n = [];
		for (let e of t) try {
			n.push($c.from(e));
		} catch (t) {
			console.log(`[Warning] Invalid Fragment ${JSON.stringify(e)}:`, t.message);
		}
		a(this, { fragments: Object.freeze(n) });
		let r = null, i = !1;
		this.#r = this.getAbiCoder(), this.fragments.forEach((e, t) => {
			let n;
			switch (e.type) {
				case "constructor":
					if (this.deploy) {
						console.log("duplicate definition - constructor");
						return;
					}
					a(this, { deploy: e });
					return;
				case "fallback":
					e.inputs.length === 0 ? i = !0 : (d(!r || e.payable !== r.payable, "conflicting fallback fragments", `fragments[${t}]`, e), r = e, i = r.payable);
					return;
				case "function":
					n = this.#n;
					break;
				case "event":
					n = this.#t;
					break;
				case "error":
					n = this.#e;
					break;
				default: return;
			}
			let o = e.format();
			n.has(o) || n.set(o, e);
		}), this.deploy || a(this, { deploy: il.from("constructor()") }), a(this, {
			fallback: r,
			receive: i
		});
	}
	format(e) {
		let t = e ? "minimal" : "full";
		return this.fragments.map((e) => e.format(t));
	}
	formatJson() {
		let e = this.fragments.map((e) => e.format("json"));
		return JSON.stringify(e.map((e) => JSON.parse(e)));
	}
	getAbiCoder() {
		return ml.defaultAbiCoder();
	}
	#i(e, t, n) {
		if (y(e)) {
			let t = e.toLowerCase();
			for (let e of this.#n.values()) if (t === e.selector) return e;
			return null;
		}
		if (e.indexOf("(") === -1) {
			let r = [];
			for (let [t, n] of this.#n) t.split("(")[0] === e && r.push(n);
			if (t) {
				let e = t.length > 0 ? t[t.length - 1] : null, n = t.length, i = !0;
				ga.isTyped(e) && e.type === "overrides" && (i = !1, n--);
				for (let e = r.length - 1; e >= 0; e--) {
					let t = r[e].inputs.length;
					t !== n && (!i || t !== n - 1) && r.splice(e, 1);
				}
				for (let e = r.length - 1; e >= 0; e--) {
					let n = r[e].inputs;
					for (let i = 0; i < t.length; i++) if (ga.isTyped(t[i])) {
						if (i >= n.length) {
							if (t[i].type === "overrides") continue;
							r.splice(e, 1);
							break;
						}
						if (t[i].type !== n[i].baseType) {
							r.splice(e, 1);
							break;
						}
					}
				}
			}
			if (r.length === 1 && t && t.length !== r[0].inputs.length) {
				let e = t[t.length - 1];
				(e == null || Array.isArray(e) || typeof e != "object") && r.splice(0, 1);
			}
			return r.length === 0 ? null : (r.length > 1 && n && d(!1, `ambiguous function description (i.e. matches ${r.map((e) => JSON.stringify(e.format())).join(", ")})`, "key", e), r[0]);
		}
		return this.#n.get(ol.from(e).format()) || null;
	}
	getFunctionName(e) {
		let t = this.#i(e, null, !1);
		return d(t, "no matching function", "key", e), t.name;
	}
	hasFunction(e) {
		return !!this.#i(e, null, !1);
	}
	getFunction(e, t) {
		return this.#i(e, t || null, !0);
	}
	forEachFunction(e) {
		let t = Array.from(this.#n.keys());
		t.sort((e, t) => e.localeCompare(t));
		for (let n = 0; n < t.length; n++) {
			let r = t[n];
			e(this.#n.get(r), n);
		}
	}
	#a(e, t, n) {
		if (y(e)) {
			let t = e.toLowerCase();
			for (let e of this.#t.values()) if (t === e.topicHash) return e;
			return null;
		}
		if (e.indexOf("(") === -1) {
			let r = [];
			for (let [t, n] of this.#t) t.split("(")[0] === e && r.push(n);
			if (t) {
				for (let e = r.length - 1; e >= 0; e--) r[e].inputs.length < t.length && r.splice(e, 1);
				for (let e = r.length - 1; e >= 0; e--) {
					let n = r[e].inputs;
					for (let i = 0; i < t.length; i++) if (ga.isTyped(t[i]) && t[i].type !== n[i].baseType) {
						r.splice(e, 1);
						break;
					}
				}
			}
			return r.length === 0 ? null : (r.length > 1 && n && d(!1, `ambiguous event description (i.e. matches ${r.map((e) => JSON.stringify(e.format())).join(", ")})`, "key", e), r[0]);
		}
		return this.#t.get(rl.from(e).format()) || null;
	}
	getEventName(e) {
		let t = this.#a(e, null, !1);
		return d(t, "no matching event", "key", e), t.name;
	}
	hasEvent(e) {
		return !!this.#a(e, null, !1);
	}
	getEvent(e, t) {
		return this.#a(e, t || null, !0);
	}
	forEachEvent(e) {
		let t = Array.from(this.#t.keys());
		t.sort((e, t) => e.localeCompare(t));
		for (let n = 0; n < t.length; n++) {
			let r = t[n];
			e(this.#t.get(r), n);
		}
	}
	getError(e, t) {
		if (y(e)) {
			let t = e.toLowerCase();
			if (bl[t]) return nl.from(bl[t].signature);
			for (let e of this.#e.values()) if (t === e.selector) return e;
			return null;
		}
		if (e.indexOf("(") === -1) {
			let t = [];
			for (let [n, r] of this.#e) n.split("(")[0] === e && t.push(r);
			return t.length === 0 ? e === "Error" ? nl.from("error Error(string)") : e === "Panic" ? nl.from("error Panic(uint256)") : null : (t.length > 1 && d(!1, `ambiguous error description (i.e. ${t.map((e) => JSON.stringify(e.format())).join(", ")})`, "name", e), t[0]);
		}
		return e = nl.from(e).format(), e === "Error(string)" ? nl.from("error Error(string)") : e === "Panic(uint256)" ? nl.from("error Panic(uint256)") : this.#e.get(e) || null;
	}
	forEachError(e) {
		let t = Array.from(this.#e.keys());
		t.sort((e, t) => e.localeCompare(t));
		for (let n = 0; n < t.length; n++) {
			let r = t[n];
			e(this.#e.get(r), n);
		}
	}
	_decodeParams(e, t) {
		return this.#r.decode(e, t);
	}
	_encodeParams(e, t) {
		return this.#r.encode(e, t);
	}
	encodeDeploy(e) {
		return this._encodeParams(this.deploy.inputs, e || []);
	}
	decodeErrorResult(e, t) {
		if (typeof e == "string") {
			let t = this.getError(e);
			d(t, "unknown error", "fragment", e), e = t;
		}
		return d(T(t, 0, 4) === e.selector, `data signature does not match error ${e.name}.`, "data", t), this._decodeParams(e.inputs, T(t, 4));
	}
	encodeErrorResult(e, t) {
		if (typeof e == "string") {
			let t = this.getError(e);
			d(t, "unknown error", "fragment", e), e = t;
		}
		return C([e.selector, this._encodeParams(e.inputs, t || [])]);
	}
	decodeFunctionData(e, t) {
		if (typeof e == "string") {
			let t = this.getFunction(e);
			d(t, "unknown function", "fragment", e), e = t;
		}
		return d(T(t, 0, 4) === e.selector, `data signature does not match function ${e.name}.`, "data", t), this._decodeParams(e.inputs, T(t, 4));
	}
	encodeFunctionData(e, t) {
		if (typeof e == "string") {
			let t = this.getFunction(e);
			d(t, "unknown function", "fragment", e), e = t;
		}
		return C([e.selector, this._encodeParams(e.inputs, t || [])]);
	}
	decodeFunctionResult(e, t) {
		if (typeof e == "string") {
			let t = this.getFunction(e);
			d(t, "unknown function", "fragment", e), e = t;
		}
		let n = "invalid length for result data", r = v(t);
		if (r.length % 32 == 0) try {
			return this.#r.decode(e.outputs, r);
		} catch {
			n = "could not decode result data";
		}
		u(!1, n, "BAD_DATA", {
			value: S(r),
			info: {
				method: e.name,
				signature: e.format()
			}
		});
	}
	makeError(e, t) {
		let n = _(e, "data"), r = ml.getBuiltinCallException("call", t, n);
		if (r.message.startsWith("execution reverted (unknown custom error)")) {
			let e = S(n.slice(0, 4)), t = this.getError(e);
			if (t) try {
				let e = this.#r.decode(t.inputs, n.slice(4));
				r.revert = {
					name: t.name,
					signature: t.format(),
					args: e
				}, r.reason = r.revert.signature, r.message = `execution reverted: ${r.reason}`;
			} catch {
				r.message = "execution reverted (coult not decode custom error)";
			}
		}
		let i = this.parseTransaction(t);
		return i && (r.invocation = {
			method: i.name,
			signature: i.signature,
			args: i.args
		}), r;
	}
	encodeFunctionResult(e, t) {
		if (typeof e == "string") {
			let t = this.getFunction(e);
			d(t, "unknown function", "fragment", e), e = t;
		}
		return S(this.#r.encode(e.outputs, t || []));
	}
	encodeFilterTopics(e, t) {
		if (typeof e == "string") {
			let t = this.getEvent(e);
			d(t, "unknown event", "eventFragment", e), e = t;
		}
		u(t.length <= e.inputs.length, `too many arguments for ${e.format()}`, "UNEXPECTED_ARGUMENT", {
			count: t.length,
			expectedCount: e.inputs.length
		});
		let n = [];
		e.anonymous || n.push(e.topicHash);
		let r = (e, t) => e.type === "string" ? go(t) : e.type === "bytes" ? W(S(t)) : (e.type === "bool" && typeof t == "boolean" ? t = t ? "0x01" : "0x00" : e.type.match(/^u?int/) ? t = te(t) : e.type.match(/^bytes/) ? t = O(t, 32) : e.type === "address" && this.#r.encode(["address"], [t]), D(S(t), 32));
		for (t.forEach((t, i) => {
			let a = e.inputs[i];
			if (!a.indexed) {
				d(t == null, "cannot filter non-indexed parameters; must be null", "contract." + a.name, t);
				return;
			}
			t == null ? n.push(null) : a.baseType === "array" || a.baseType === "tuple" ? d(!1, "filtering with tuples or arrays not supported", "contract." + a.name, t) : Array.isArray(t) ? n.push(t.map((e) => r(a, e))) : n.push(r(a, t));
		}); n.length && n[n.length - 1] === null;) n.pop();
		return n;
	}
	encodeEventLog(e, t) {
		if (typeof e == "string") {
			let t = this.getEvent(e);
			d(t, "unknown event", "eventFragment", e), e = t;
		}
		let n = [], r = [], i = [];
		return e.anonymous || n.push(e.topicHash), d(t.length === e.inputs.length, "event arguments/values mismatch", "values", t), e.inputs.forEach((e, a) => {
			let o = t[a];
			if (e.indexed) {
				if (e.type === "string") n.push(go(o));
				else if (e.type === "bytes") n.push(W(o));
				else if (e.baseType === "tuple" || e.baseType === "array") throw Error("not implemented");
				else n.push(this.#r.encode([e.type], [o]));
			} else r.push(e), i.push(o);
		}), {
			data: this.#r.encode(r, i),
			topics: n
		};
	}
	decodeEventLog(e, t, n) {
		if (typeof e == "string") {
			let t = this.getEvent(e);
			d(t, "unknown event", "eventFragment", e), e = t;
		}
		if (n != null && !e.anonymous) {
			let t = e.topicHash;
			d(y(n[0], 32) && n[0].toLowerCase() === t, "fragment/topic mismatch", "topics[0]", n[0]), n = n.slice(1);
		}
		let r = [], i = [], a = [];
		e.inputs.forEach((e, t) => {
			e.indexed ? e.type === "string" || e.type === "bytes" || e.baseType === "tuple" || e.baseType === "array" ? (r.push(Qc.from({
				type: "bytes32",
				name: e.name
			})), a.push(!0)) : (r.push(e), a.push(!1)) : (i.push(e), a.push(!1));
		});
		let o = n == null ? null : this.#r.decode(r, C(n)), s = this.#r.decode(i, t, !0), c = [], l = [], u = 0, f = 0;
		return e.inputs.forEach((e, t) => {
			let n = null;
			if (e.indexed) {
				if (o == null) n = new vl(null);
				else if (a[t]) n = new vl(o[f++]);
				else try {
					n = o[f++];
				} catch (e) {
					n = e;
				}
			} else try {
				n = s[u++];
			} catch (e) {
				n = e;
			}
			c.push(n), l.push(e.name || null);
		}), dt.fromItems(c, l);
	}
	parseTransaction(e) {
		let t = _(e.data, "tx.data"), n = F(e.value == null ? 0 : e.value, "tx.value"), r = this.getFunction(S(t.slice(0, 4)));
		if (!r) return null;
		let i = this.#r.decode(r.inputs, t.slice(4));
		return new gl(r, r.selector, i, n);
	}
	parseCallResult(e) {
		throw Error("@TODO");
	}
	parseLog(e) {
		let t = this.getEvent(e.topics[0]);
		return !t || t.anonymous ? null : new hl(t, t.topicHash, this.decodeEventLog(t, e.data, e.topics));
	}
	parseError(e) {
		let t = S(e), n = this.getError(T(t, 0, 4));
		if (!n) return null;
		let r = this.#r.decode(n.inputs, T(t, 4));
		return new _l(n, n.selector, r);
	}
	static from(t) {
		return t instanceof e ? t : typeof t == "string" ? new e(JSON.parse(t)) : typeof t.formatJson == "function" ? new e(t.formatJson()) : typeof t.format == "function" ? new e(t.format("json")) : new e(t);
	}
}, Sl = BigInt(0);
function Cl(e) {
	return e ?? null;
}
function wl(e) {
	return e == null ? null : e.toString();
}
var Tl = class {
	gasPrice;
	maxFeePerGas;
	maxPriorityFeePerGas;
	constructor(e, t, n) {
		a(this, {
			gasPrice: Cl(e),
			maxFeePerGas: Cl(t),
			maxPriorityFeePerGas: Cl(n)
		});
	}
	toJSON() {
		let { gasPrice: e, maxFeePerGas: t, maxPriorityFeePerGas: n } = this;
		return {
			_type: "FeeData",
			gasPrice: wl(e),
			maxFeePerGas: wl(t),
			maxPriorityFeePerGas: wl(n)
		};
	}
};
function El(e) {
	let t = {};
	e.to && (t.to = e.to), e.from && (t.from = e.from), e.data && (t.data = S(e.data));
	let n = "chainId,gasLimit,gasPrice,maxFeePerBlobGas,maxFeePerGas,maxPriorityFeePerGas,value".split(/,/);
	for (let r of n) r in e && e[r] != null && (t[r] = F(e[r], `request.${r}`));
	let r = "type,nonce".split(/,/);
	for (let n of r) n in e && e[n] != null && (t[n] = z(e[n], `request.${n}`));
	return e.accessList && (t.accessList = Fa(e.accessList)), e.authorizationList && (t.authorizationList = e.authorizationList.slice()), "blockTag" in e && (t.blockTag = e.blockTag), "enableCcipRead" in e && (t.enableCcipRead = !!e.enableCcipRead), "customData" in e && (t.customData = e.customData), "blobVersionedHashes" in e && e.blobVersionedHashes && (t.blobVersionedHashes = e.blobVersionedHashes.slice()), "kzg" in e && (t.kzg = e.kzg), "blobWrapperVersion" in e && (t.blobWrapperVersion = e.blobWrapperVersion), "blobs" in e && e.blobs && (t.blobs = e.blobs.map((e) => b(e) ? S(e) : Object.assign({}, e))), t;
}
var Dl = class {
	provider;
	number;
	hash;
	timestamp;
	parentHash;
	parentBeaconBlockRoot;
	nonce;
	difficulty;
	gasLimit;
	gasUsed;
	stateRoot;
	receiptsRoot;
	transactionsRoot;
	blobGasUsed;
	excessBlobGas;
	miner;
	prevRandao;
	extraData;
	baseFeePerGas;
	#e;
	constructor(e, t) {
		this.#e = e.transactions.map((e) => typeof e == "string" ? e : new Al(e, t)), a(this, {
			provider: t,
			hash: Cl(e.hash),
			number: e.number,
			timestamp: e.timestamp,
			parentHash: e.parentHash,
			parentBeaconBlockRoot: e.parentBeaconBlockRoot,
			nonce: e.nonce,
			difficulty: e.difficulty,
			gasLimit: e.gasLimit,
			gasUsed: e.gasUsed,
			blobGasUsed: e.blobGasUsed,
			excessBlobGas: e.excessBlobGas,
			miner: e.miner,
			prevRandao: Cl(e.prevRandao),
			extraData: e.extraData,
			baseFeePerGas: Cl(e.baseFeePerGas),
			stateRoot: e.stateRoot,
			receiptsRoot: e.receiptsRoot,
			transactionsRoot: e.transactionsRoot
		});
	}
	get transactions() {
		return this.#e.map((e) => typeof e == "string" ? e : e.hash);
	}
	get prefetchedTransactions() {
		let e = this.#e.slice();
		return e.length === 0 ? [] : (u(typeof e[0] == "object", "transactions were not prefetched with block request", "UNSUPPORTED_OPERATION", { operation: "transactionResponses()" }), e);
	}
	toJSON() {
		let { baseFeePerGas: e, difficulty: t, extraData: n, gasLimit: r, gasUsed: i, hash: a, miner: o, prevRandao: s, nonce: c, number: l, parentHash: u, parentBeaconBlockRoot: d, stateRoot: f, receiptsRoot: p, transactionsRoot: m, timestamp: h, transactions: g } = this;
		return {
			_type: "Block",
			baseFeePerGas: wl(e),
			difficulty: wl(t),
			extraData: n,
			gasLimit: wl(r),
			gasUsed: wl(i),
			blobGasUsed: wl(this.blobGasUsed),
			excessBlobGas: wl(this.excessBlobGas),
			hash: a,
			miner: o,
			prevRandao: s,
			nonce: c,
			number: l,
			parentHash: u,
			timestamp: h,
			parentBeaconBlockRoot: d,
			stateRoot: f,
			receiptsRoot: p,
			transactionsRoot: m,
			transactions: g
		};
	}
	[Symbol.iterator]() {
		let e = 0, t = this.transactions;
		return { next: () => e < this.length ? {
			value: t[e++],
			done: !1
		} : {
			value: void 0,
			done: !0
		} };
	}
	get length() {
		return this.#e.length;
	}
	get date() {
		return this.timestamp == null ? null : /* @__PURE__ */ new Date(this.timestamp * 1e3);
	}
	async getTransaction(e) {
		let t;
		if (typeof e == "number") t = this.#e[e];
		else {
			let n = e.toLowerCase();
			for (let e of this.#e) if (typeof e == "string") {
				if (e !== n) continue;
				t = e;
				break;
			} else {
				if (e.hash !== n) continue;
				t = e;
				break;
			}
		}
		if (t == null) throw Error("no such tx");
		return typeof t == "string" ? await this.provider.getTransaction(t) : t;
	}
	getPrefetchedTransaction(e) {
		let t = this.prefetchedTransactions;
		if (typeof e == "number") return t[e];
		e = e.toLowerCase();
		for (let n of t) if (n.hash === e) return n;
		d(!1, "no matching transaction", "indexOrHash", e);
	}
	isMined() {
		return !!this.hash;
	}
	isLondon() {
		return !!this.baseFeePerGas;
	}
	orphanedEvent() {
		if (!this.isMined()) throw Error("");
		return jl(this);
	}
}, Ol = class {
	provider;
	transactionHash;
	blockHash;
	blockNumber;
	removed;
	address;
	data;
	topics;
	index;
	transactionIndex;
	constructor(e, t) {
		this.provider = t;
		let n = Object.freeze(e.topics.slice());
		a(this, {
			transactionHash: e.transactionHash,
			blockHash: e.blockHash,
			blockNumber: e.blockNumber,
			removed: e.removed,
			address: e.address,
			data: e.data,
			topics: n,
			index: e.index,
			transactionIndex: e.transactionIndex
		});
	}
	toJSON() {
		let { address: e, blockHash: t, blockNumber: n, data: r, index: i, removed: a, topics: o, transactionHash: s, transactionIndex: c } = this;
		return {
			_type: "log",
			address: e,
			blockHash: t,
			blockNumber: n,
			data: r,
			index: i,
			removed: a,
			topics: o,
			transactionHash: s,
			transactionIndex: c
		};
	}
	async getBlock() {
		let e = await this.provider.getBlock(this.blockHash);
		return u(!!e, "failed to find transaction", "UNKNOWN_ERROR", {}), e;
	}
	async getTransaction() {
		let e = await this.provider.getTransaction(this.transactionHash);
		return u(!!e, "failed to find transaction", "UNKNOWN_ERROR", {}), e;
	}
	async getTransactionReceipt() {
		let e = await this.provider.getTransactionReceipt(this.transactionHash);
		return u(!!e, "failed to find transaction receipt", "UNKNOWN_ERROR", {}), e;
	}
	removedEvent() {
		return Pl(this);
	}
}, kl = class {
	provider;
	to;
	from;
	contractAddress;
	hash;
	index;
	blockHash;
	blockNumber;
	logsBloom;
	gasUsed;
	blobGasUsed;
	cumulativeGasUsed;
	gasPrice;
	blobGasPrice;
	type;
	status;
	root;
	#e;
	constructor(e, t) {
		this.#e = Object.freeze(e.logs.map((e) => new Ol(e, t)));
		let n = Sl;
		e.effectiveGasPrice == null ? e.gasPrice != null && (n = e.gasPrice) : n = e.effectiveGasPrice, a(this, {
			provider: t,
			to: e.to,
			from: e.from,
			contractAddress: e.contractAddress,
			hash: e.hash,
			index: e.index,
			blockHash: e.blockHash,
			blockNumber: e.blockNumber,
			logsBloom: e.logsBloom,
			gasUsed: e.gasUsed,
			cumulativeGasUsed: e.cumulativeGasUsed,
			blobGasUsed: e.blobGasUsed,
			gasPrice: n,
			blobGasPrice: e.blobGasPrice,
			type: e.type,
			status: e.status,
			root: e.root
		});
	}
	get logs() {
		return this.#e;
	}
	toJSON() {
		let { to: e, from: t, contractAddress: n, hash: r, index: i, blockHash: a, blockNumber: o, logsBloom: s, logs: c, status: l, root: u } = this;
		return {
			_type: "TransactionReceipt",
			blockHash: a,
			blockNumber: o,
			contractAddress: n,
			cumulativeGasUsed: wl(this.cumulativeGasUsed),
			from: t,
			gasPrice: wl(this.gasPrice),
			blobGasUsed: wl(this.blobGasUsed),
			blobGasPrice: wl(this.blobGasPrice),
			gasUsed: wl(this.gasUsed),
			hash: r,
			index: i,
			logs: c,
			logsBloom: s,
			root: u,
			status: l,
			to: e
		};
	}
	get length() {
		return this.logs.length;
	}
	[Symbol.iterator]() {
		let e = 0;
		return { next: () => e < this.length ? {
			value: this.logs[e++],
			done: !1
		} : {
			value: void 0,
			done: !0
		} };
	}
	get fee() {
		return this.gasUsed * this.gasPrice;
	}
	async getBlock() {
		let e = await this.provider.getBlock(this.blockHash);
		if (e == null) throw Error("TODO");
		return e;
	}
	async getTransaction() {
		let e = await this.provider.getTransaction(this.hash);
		if (e == null) throw Error("TODO");
		return e;
	}
	async getResult() {
		return await this.provider.getTransactionResult(this.hash);
	}
	async confirmations() {
		return await this.provider.getBlockNumber() - this.blockNumber + 1;
	}
	removedEvent() {
		return Nl(this);
	}
	reorderedEvent(e) {
		return u(!e || e.isMined(), "unmined 'other' transction cannot be orphaned", "UNSUPPORTED_OPERATION", { operation: "reorderedEvent(other)" }), Ml(this, e);
	}
}, Al = class e {
	provider;
	blockNumber;
	blockHash;
	index;
	hash;
	type;
	to;
	from;
	nonce;
	gasLimit;
	gasPrice;
	maxPriorityFeePerGas;
	maxFeePerGas;
	maxFeePerBlobGas;
	data;
	value;
	chainId;
	signature;
	accessList;
	blobVersionedHashes;
	authorizationList;
	#e;
	constructor(e, t) {
		this.provider = t, this.blockNumber = e.blockNumber == null ? null : e.blockNumber, this.blockHash = e.blockHash == null ? null : e.blockHash, this.hash = e.hash, this.index = e.index, this.type = e.type, this.from = e.from, this.to = e.to || null, this.gasLimit = e.gasLimit, this.nonce = e.nonce, this.data = e.data, this.value = e.value, this.gasPrice = e.gasPrice, this.maxPriorityFeePerGas = e.maxPriorityFeePerGas == null ? null : e.maxPriorityFeePerGas, this.maxFeePerGas = e.maxFeePerGas == null ? null : e.maxFeePerGas, this.maxFeePerBlobGas = e.maxFeePerBlobGas == null ? null : e.maxFeePerBlobGas, this.chainId = e.chainId, this.signature = e.signature, this.accessList = e.accessList == null ? null : e.accessList, this.blobVersionedHashes = e.blobVersionedHashes == null ? null : e.blobVersionedHashes, this.authorizationList = e.authorizationList == null ? null : e.authorizationList, this.#e = -1;
	}
	toJSON() {
		let { blockNumber: e, blockHash: t, index: n, hash: r, type: i, to: a, from: o, nonce: s, data: c, signature: l, accessList: u, blobVersionedHashes: d } = this;
		return {
			_type: "TransactionResponse",
			accessList: u,
			blockNumber: e,
			blockHash: t,
			blobVersionedHashes: d,
			chainId: wl(this.chainId),
			data: c,
			from: o,
			gasLimit: wl(this.gasLimit),
			gasPrice: wl(this.gasPrice),
			hash: r,
			maxFeePerGas: wl(this.maxFeePerGas),
			maxPriorityFeePerGas: wl(this.maxPriorityFeePerGas),
			maxFeePerBlobGas: wl(this.maxFeePerBlobGas),
			nonce: s,
			signature: l,
			to: a,
			index: n,
			type: i,
			value: wl(this.value)
		};
	}
	async getBlock() {
		let e = this.blockNumber;
		if (e == null) {
			let t = await this.getTransaction();
			t && (e = t.blockNumber);
		}
		if (e == null) return null;
		let t = this.provider.getBlock(e);
		if (t == null) throw Error("TODO");
		return t;
	}
	async getTransaction() {
		return this.provider.getTransaction(this.hash);
	}
	async confirmations() {
		if (this.blockNumber == null) {
			let { tx: e, blockNumber: t } = await i({
				tx: this.getTransaction(),
				blockNumber: this.provider.getBlockNumber()
			});
			return e == null || e.blockNumber == null ? 0 : t - e.blockNumber + 1;
		}
		return await this.provider.getBlockNumber() - this.blockNumber + 1;
	}
	async wait(e, t) {
		let n = e ?? 1, r = t ?? 0, a = this.#e, o = -1, c = a === -1, d = async () => {
			if (c) return null;
			let { blockNumber: e, nonce: t } = await i({
				blockNumber: this.provider.getBlockNumber(),
				nonce: this.provider.getTransactionCount(this.from)
			});
			if (t < this.nonce) {
				a = e;
				return;
			}
			if (c) return null;
			let r = await this.getTransaction();
			if (!(r && r.blockNumber != null)) for (o === -1 && (o = a - 3, o < this.#e && (o = this.#e)); o <= e;) {
				if (c) return null;
				let t = await this.provider.getBlock(o, !0);
				if (t == null) return;
				for (let e of t) if (e === this.hash) return;
				for (let r = 0; r < t.length; r++) {
					let i = await t.getTransaction(r);
					if (i.from === this.from && i.nonce === this.nonce) {
						if (c) return null;
						let t = await this.provider.getTransactionReceipt(i.hash);
						if (t == null || e - t.blockNumber + 1 < n) return;
						let r = "replaced";
						i.data === this.data && i.to === this.to && i.value === this.value ? r = "repriced" : i.data === "0x" && i.from === i.to && i.value === Sl && (r = "cancelled"), u(!1, "transaction was replaced", "TRANSACTION_REPLACED", {
							cancelled: r === "replaced" || r === "cancelled",
							reason: r,
							replacement: i.replaceableTransaction(a),
							hash: i.hash,
							receipt: t
						});
					}
				}
				o++;
			}
		}, f = (e) => {
			if (e == null || e.status !== 0) return e;
			u(!1, "transaction execution reverted", "CALL_EXCEPTION", {
				action: "sendTransaction",
				data: null,
				reason: null,
				invocation: null,
				revert: null,
				transaction: {
					to: e.to,
					from: e.from,
					data: ""
				},
				receipt: e
			});
		}, p = await this.provider.getTransactionReceipt(this.hash);
		if (n === 0) return f(p);
		if (p) {
			if (n === 1 || await p.confirmations() >= n) return f(p);
		} else if (await d(), n === 0) return null;
		return await new Promise((e, t) => {
			let i = [], o = () => {
				i.forEach((e) => e());
			};
			if (i.push(() => {
				c = !0;
			}), r > 0) {
				let e = setTimeout(() => {
					o(), t(l("wait for transaction timeout", "TIMEOUT"));
				}, r);
				i.push(() => {
					clearTimeout(e);
				});
			}
			let u = async (r) => {
				if (await r.confirmations() >= n) {
					o();
					try {
						e(f(r));
					} catch (e) {
						t(e);
					}
				}
			};
			if (i.push(() => {
				this.provider.off(this.hash, u);
			}), this.provider.on(this.hash, u), a >= 0) {
				let e = async () => {
					try {
						await d();
					} catch (e) {
						if (s(e, "TRANSACTION_REPLACED")) {
							o(), t(e);
							return;
						}
					}
					c || this.provider.once("block", e);
				};
				i.push(() => {
					this.provider.off("block", e);
				}), this.provider.once("block", e);
			}
		});
	}
	isMined() {
		return this.blockHash != null;
	}
	isLegacy() {
		return this.type === 0;
	}
	isBerlin() {
		return this.type === 1;
	}
	isLondon() {
		return this.type === 2;
	}
	isCancun() {
		return this.type === 3;
	}
	removedEvent() {
		return u(this.isMined(), "unmined transaction canot be orphaned", "UNSUPPORTED_OPERATION", { operation: "removeEvent()" }), Nl(this);
	}
	reorderedEvent(e) {
		return u(this.isMined(), "unmined transaction canot be orphaned", "UNSUPPORTED_OPERATION", { operation: "removeEvent()" }), u(!e || e.isMined(), "unmined 'other' transaction canot be orphaned", "UNSUPPORTED_OPERATION", { operation: "removeEvent()" }), Ml(this, e);
	}
	replaceableTransaction(t) {
		d(Number.isInteger(t) && t >= 0, "invalid startBlock", "startBlock", t);
		let n = new e(this, this.provider);
		return n.#e = t, n;
	}
};
function jl(e) {
	return {
		orphan: "drop-block",
		hash: e.hash,
		number: e.number
	};
}
function Ml(e, t) {
	return {
		orphan: "reorder-transaction",
		tx: e,
		other: t
	};
}
function Nl(e) {
	return {
		orphan: "drop-transaction",
		tx: e
	};
}
function Pl(e) {
	return {
		orphan: "drop-log",
		log: {
			transactionHash: e.transactionHash,
			blockHash: e.blockHash,
			blockNumber: e.blockNumber,
			address: e.address,
			data: e.data,
			topics: Object.freeze(e.topics.slice()),
			index: e.index
		}
	};
}
//#endregion
//#region node_modules/ethers/lib.esm/contract/wrappers.js
var Fl = class extends Ol {
	interface;
	fragment;
	args;
	constructor(e, t, n) {
		super(e, e.provider);
		let r = t.decodeEventLog(n, e.data, e.topics);
		a(this, {
			args: r,
			fragment: n,
			interface: t
		});
	}
	get eventName() {
		return this.fragment.name;
	}
	get eventSignature() {
		return this.fragment.format();
	}
}, Il = class extends Ol {
	error;
	constructor(e, t) {
		super(e, e.provider), a(this, { error: t });
	}
}, Ll = class extends kl {
	#e;
	constructor(e, t, n) {
		super(n, t), this.#e = e;
	}
	get logs() {
		return super.logs.map((e) => {
			let t = e.topics.length ? this.#e.getEvent(e.topics[0]) : null;
			if (t) try {
				return new Fl(e, this.#e, t);
			} catch (t) {
				return new Il(e, t);
			}
			return e;
		});
	}
}, Rl = class extends Al {
	#e;
	constructor(e, t, n) {
		super(n, t), this.#e = e;
	}
	async wait(e, t) {
		let n = await super.wait(e, t);
		return n == null ? null : new Ll(this.#e, this.provider, n);
	}
}, zl = class extends se {
	log;
	constructor(e, t, n, r) {
		super(e, t, n), a(this, { log: r });
	}
	async getBlock() {
		return await this.log.getBlock();
	}
	async getTransaction() {
		return await this.log.getTransaction();
	}
	async getTransactionReceipt() {
		return await this.log.getTransactionReceipt();
	}
}, Bl = class extends zl {
	constructor(e, t, n, r, i) {
		super(e, t, n, new Fl(i, e.interface, r));
		let o = e.interface.decodeEventLog(r, this.log.data, this.log.topics);
		a(this, {
			args: o,
			fragment: r
		});
	}
	get eventName() {
		return this.fragment.name;
	}
	get eventSignature() {
		return this.fragment.format();
	}
}, Vl = BigInt(0);
function Hl(e) {
	return e && typeof e.call == "function";
}
function Ul(e) {
	return e && typeof e.estimateGas == "function";
}
function Wl(e) {
	return e && typeof e.resolveName == "function";
}
function Gl(e) {
	return e && typeof e.sendTransaction == "function";
}
function Kl(e) {
	if (e != null) {
		if (Wl(e)) return e;
		if (e.provider) return e.provider;
	}
}
var ql = class {
	#e;
	fragment;
	constructor(e, t, n) {
		if (a(this, { fragment: t }), t.inputs.length < n.length) throw Error("too many arguments");
		let r = Jl(e.runner, "resolveName"), i = Wl(r) ? r : null;
		this.#e = (async function() {
			let r = await Promise.all(t.inputs.map((e, t) => n[t] == null ? null : e.walkAsync(n[t], (e, t) => e === "address" ? Array.isArray(t) ? Promise.all(t.map((e) => pa(e, i))) : pa(t, i) : t)));
			return e.interface.encodeFilterTopics(t, r);
		})();
	}
	getTopicFilter() {
		return this.#e;
	}
};
function Jl(e, t) {
	return e == null ? null : typeof e[t] == "function" ? e : e.provider && typeof e.provider[t] == "function" ? e.provider : null;
}
function Yl(e) {
	return e == null ? null : e.provider || null;
}
async function Xl(e, t) {
	let n = ga.dereference(e, "overrides");
	d(typeof n == "object", "invalid overrides parameter", "overrides", e);
	let r = El(n);
	return d(r.to == null || (t || []).indexOf("to") >= 0, "cannot override to", "overrides.to", r.to), d(r.data == null || (t || []).indexOf("data") >= 0, "cannot override data", "overrides.data", r.data), r.from &&= r.from, r;
}
async function Zl(e, t, n) {
	let r = Jl(e, "resolveName"), i = Wl(r) ? r : null;
	return await Promise.all(t.map((e, t) => e.walkAsync(n[t], (e, t) => (t = ga.dereference(t, e), e === "address" ? pa(t, i) : t))));
}
function Ql(e) {
	let t = async function(t) {
		let n = await Xl(t, ["data"]);
		n.to = await e.getAddress(), n.from &&= await pa(n.from, Kl(e.runner));
		let r = e.interface, i = F(n.value || Vl, "overrides.value") === Vl, a = (n.data || "0x") === "0x";
		return r.fallback && !r.fallback.payable && r.receive && !a && !i && d(!1, "cannot send data to receive or send value to non-payable fallback", "overrides", t), d(r.fallback || a, "cannot send data to receive-only contract", "overrides.data", n.data), d(r.receive || r.fallback && r.fallback.payable || i, "cannot send value to non-payable fallback", "overrides.value", n.value), d(r.fallback || a, "cannot send data to receive-only contract", "overrides.data", n.data), n;
	}, n = async function(n) {
		let r = Jl(e.runner, "call");
		u(Hl(r), "contract runner does not support calling", "UNSUPPORTED_OPERATION", { operation: "call" });
		let i = await t(n);
		try {
			return await r.call(i);
		} catch (t) {
			throw c(t) && t.data ? e.interface.makeError(t.data, i) : t;
		}
	}, r = async function(n) {
		let r = e.runner;
		u(Gl(r), "contract runner does not support sending transactions", "UNSUPPORTED_OPERATION", { operation: "sendTransaction" });
		let i = await r.sendTransaction(await t(n)), a = Yl(e.runner);
		return new Rl(e.interface, a, i);
	}, i = async function(n) {
		let r = Jl(e.runner, "estimateGas");
		return u(Ul(r), "contract runner does not support gas estimation", "UNSUPPORTED_OPERATION", { operation: "estimateGas" }), await r.estimateGas(await t(n));
	}, o = async (e) => await r(e);
	return a(o, {
		_contract: e,
		estimateGas: i,
		populateTransaction: t,
		send: r,
		staticCall: n
	}), o;
}
function $l(e, t) {
	let n = function(...n) {
		let r = e.interface.getFunction(t, n);
		return u(r, "no matching fragment", "UNSUPPORTED_OPERATION", {
			operation: "fragment",
			info: {
				key: t,
				args: n
			}
		}), r;
	}, r = async function(...t) {
		let r = n(...t), a = {};
		if (r.inputs.length + 1 === t.length && (a = await Xl(t.pop()), a.from && (a.from = await pa(a.from, Kl(e.runner)))), r.inputs.length !== t.length) throw Error("internal error: fragment inputs doesn't match arguments; should not happen");
		let o = await Zl(e.runner, r.inputs, t);
		return Object.assign({}, a, await i({
			to: e.getAddress(),
			data: e.interface.encodeFunctionData(r, o)
		}));
	}, o = async function(...e) {
		let t = await d(...e);
		return t.length === 1 ? t[0] : t;
	}, s = async function(...t) {
		let n = e.runner;
		u(Gl(n), "contract runner does not support sending transactions", "UNSUPPORTED_OPERATION", { operation: "sendTransaction" });
		let i = await n.sendTransaction(await r(...t)), a = Yl(e.runner);
		return new Rl(e.interface, a, i);
	}, l = async function(...t) {
		let n = Jl(e.runner, "estimateGas");
		return u(Ul(n), "contract runner does not support gas estimation", "UNSUPPORTED_OPERATION", { operation: "estimateGas" }), await n.estimateGas(await r(...t));
	}, d = async function(...t) {
		let i = Jl(e.runner, "call");
		u(Hl(i), "contract runner does not support calling", "UNSUPPORTED_OPERATION", { operation: "call" });
		let a = await r(...t), o = "0x";
		try {
			o = await i.call(a);
		} catch (t) {
			throw c(t) && t.data ? e.interface.makeError(t.data, a) : t;
		}
		let s = n(...t);
		return e.interface.decodeFunctionResult(s, o);
	}, f = async (...e) => n(...e).constant ? await o(...e) : await s(...e);
	return a(f, {
		name: e.interface.getFunctionName(t),
		_contract: e,
		_key: t,
		getFragment: n,
		estimateGas: l,
		populateTransaction: r,
		send: s,
		staticCall: o,
		staticCallResult: d
	}), Object.defineProperty(f, "fragment", {
		configurable: !1,
		enumerable: !0,
		get: () => {
			let n = e.interface.getFunction(t);
			return u(n, "no matching fragment", "UNSUPPORTED_OPERATION", {
				operation: "fragment",
				info: { key: t }
			}), n;
		}
	}), f;
}
function eu(e, t) {
	let n = function(...n) {
		let r = e.interface.getEvent(t, n);
		return u(r, "no matching fragment", "UNSUPPORTED_OPERATION", {
			operation: "fragment",
			info: {
				key: t,
				args: n
			}
		}), r;
	}, r = function(...t) {
		return new ql(e, n(...t), t);
	};
	return a(r, {
		name: e.interface.getEventName(t),
		_contract: e,
		_key: t,
		getFragment: n
	}), Object.defineProperty(r, "fragment", {
		configurable: !1,
		enumerable: !0,
		get: () => {
			let n = e.interface.getEvent(t);
			return u(n, "no matching fragment", "UNSUPPORTED_OPERATION", {
				operation: "fragment",
				info: { key: t }
			}), n;
		}
	}), r;
}
var tu = Symbol.for("_ethersInternal_contract"), nu = /* @__PURE__ */ new WeakMap();
function ru(e, t) {
	nu.set(e[tu], t);
}
function iu(e) {
	return nu.get(e[tu]);
}
function au(e) {
	return e && typeof e == "object" && "getTopicFilter" in e && typeof e.getTopicFilter == "function" && e.fragment;
}
async function ou(e, t) {
	let n, r = null;
	if (Array.isArray(t)) {
		let r = function(t) {
			if (y(t, 32)) return t;
			let n = e.interface.getEvent(t);
			return d(n, "unknown fragment", "name", t), n.topicHash;
		};
		n = t.map((e) => e == null ? null : Array.isArray(e) ? e.map(r) : r(e));
	} else t === "*" ? n = [null] : typeof t == "string" ? y(t, 32) ? n = [t] : (r = e.interface.getEvent(t), d(r, "unknown fragment", "event", t), n = [r.topicHash]) : au(t) ? n = await t.getTopicFilter() : "fragment" in t ? (r = t.fragment, n = [r.topicHash]) : d(!1, "unknown event name", "event", t);
	n = n.map((e) => {
		if (e == null) return null;
		if (Array.isArray(e)) {
			let t = Array.from(new Set(e.map((e) => e.toLowerCase())).values());
			return t.length === 1 ? t[0] : (t.sort(), t);
		}
		return e.toLowerCase();
	});
	let i = n.map((e) => e == null ? "null" : Array.isArray(e) ? e.join("|") : e).join("&");
	return {
		fragment: r,
		tag: i,
		topics: n
	};
}
async function su(e, t) {
	let { subs: n } = iu(e);
	return n.get((await ou(e, t)).tag) || null;
}
async function cu(e, t, n) {
	let r = Yl(e.runner);
	u(r, "contract runner does not support subscribing", "UNSUPPORTED_OPERATION", { operation: t });
	let { fragment: i, tag: a, topics: o } = await ou(e, n), { addr: s, subs: c } = iu(e), l = c.get(a);
	if (!l) {
		let t = {
			address: s || e,
			topics: o
		}, u = (t) => {
			let r = i;
			if (r == null) try {
				r = e.interface.getEvent(t.topics[0]);
			} catch {}
			if (r) {
				let a = r;
				du(e, n, i ? e.interface.decodeEventLog(i, t.data, t.topics) : [], (r) => new Bl(e, r, n, a, t));
			} else du(e, n, [], (r) => new zl(e, r, n, t));
		}, d = [];
		l = {
			tag: a,
			listeners: [],
			start: () => {
				d.length || d.push(r.on(t, u));
			},
			stop: async () => {
				if (d.length == 0) return;
				let e = d;
				d = [], await Promise.all(e), r.off(t, u);
			}
		}, c.set(a, l);
	}
	return l;
}
var lu = Promise.resolve();
async function uu(e, t, n, r) {
	await lu;
	let i = await su(e, t);
	if (!i) return !1;
	let a = i.listeners.length;
	return i.listeners = i.listeners.filter(({ listener: t, once: i }) => {
		let a = Array.from(n);
		r && a.push(r(i ? null : t));
		try {
			t.call(e, ...a);
		} catch {}
		return !i;
	}), i.listeners.length === 0 && (i.stop(), iu(e).subs.delete(i.tag)), a > 0;
}
async function du(e, t, n, r) {
	try {
		await lu;
	} catch {}
	let i = uu(e, t, n, r);
	return lu = i, await i;
}
var fu = ["then"], pu = class e {
	target;
	interface;
	runner;
	filters;
	[tu];
	fallback;
	constructor(e, t, n, r) {
		d(typeof e == "string" || da(e), "invalid value for Contract target", "target", e), n ??= null;
		let i = xl.from(t);
		a(this, {
			target: e,
			runner: n,
			interface: i
		}), Object.defineProperty(this, tu, { value: {} });
		let o, c = null, u = null;
		if (r) {
			let e = Yl(n);
			u = new Rl(this.interface, e, r);
		}
		let f = /* @__PURE__ */ new Map();
		if (typeof e == "string") {
			if (y(e)) c = e, o = Promise.resolve(e);
			else {
				let t = Jl(n, "resolveName");
				if (!Wl(t)) throw l("contract runner does not support name resolution", "UNSUPPORTED_OPERATION", { operation: "resolveName" });
				o = t.resolveName(e).then((t) => {
					if (t == null) throw l("an ENS name used for a contract target must be correctly configured", "UNCONFIGURED_NAME", { value: e });
					return iu(this).addr = t, t;
				});
			}
		} else o = e.getAddress().then((e) => {
			if (e == null) throw Error("TODO");
			return iu(this).addr = e, e;
		});
		ru(this, {
			addrPromise: o,
			addr: c,
			deployTx: u,
			subs: f
		});
		let p = new Proxy({}, {
			get: (e, t, n) => {
				if (typeof t == "symbol" || fu.indexOf(t) >= 0) return Reflect.get(e, t, n);
				try {
					return this.getEvent(t);
				} catch (e) {
					if (!s(e, "INVALID_ARGUMENT") || e.argument !== "key") throw e;
				}
			},
			has: (e, t) => fu.indexOf(t) >= 0 ? Reflect.has(e, t) : Reflect.has(e, t) || this.interface.hasEvent(String(t))
		});
		return a(this, { filters: p }), a(this, { fallback: i.receive || i.fallback ? Ql(this) : null }), new Proxy(this, {
			get: (e, t, n) => {
				if (typeof t == "symbol" || t in e || fu.indexOf(t) >= 0) return Reflect.get(e, t, n);
				try {
					return e.getFunction(t);
				} catch (e) {
					if (!s(e, "INVALID_ARGUMENT") || e.argument !== "key") throw e;
				}
			},
			has: (e, t) => typeof t == "symbol" || t in e || fu.indexOf(t) >= 0 ? Reflect.has(e, t) : e.interface.hasFunction(t)
		});
	}
	connect(t) {
		return new e(this.target, this.interface, t);
	}
	attach(t) {
		return new e(t, this.interface, this.runner);
	}
	async getAddress() {
		return await iu(this).addrPromise;
	}
	async getDeployedCode() {
		let e = Yl(this.runner);
		u(e, "runner does not support .provider", "UNSUPPORTED_OPERATION", { operation: "getDeployedCode" });
		let t = await e.getCode(await this.getAddress());
		return t === "0x" ? null : t;
	}
	async waitForDeployment() {
		let e = this.deploymentTransaction();
		if (e) return await e.wait(), this;
		if (await this.getDeployedCode() != null) return this;
		let t = Yl(this.runner);
		return u(t != null, "contract runner does not support .provider", "UNSUPPORTED_OPERATION", { operation: "waitForDeployment" }), new Promise((e, n) => {
			let r = async () => {
				try {
					if (await this.getDeployedCode() != null) return e(this);
					t.once("block", r);
				} catch (e) {
					n(e);
				}
			};
			r();
		});
	}
	deploymentTransaction() {
		return iu(this).deployTx;
	}
	getFunction(e) {
		return typeof e != "string" && (e = e.format()), $l(this, e);
	}
	getEvent(e) {
		return typeof e != "string" && (e = e.format()), eu(this, e);
	}
	async queryTransaction(e) {
		throw Error("@TODO");
	}
	async queryFilter(e, t, n) {
		t ??= 0, n ??= "latest";
		let { addr: r, addrPromise: i } = iu(this), a = r || await i, { fragment: o, topics: s } = await ou(this, e), c = {
			address: a,
			topics: s,
			fromBlock: t,
			toBlock: n
		}, l = Yl(this.runner);
		return u(l, "contract runner does not have a provider", "UNSUPPORTED_OPERATION", { operation: "queryFilter" }), (await l.getLogs(c)).map((e) => {
			let t = o;
			if (t == null) try {
				t = this.interface.getEvent(e.topics[0]);
			} catch {}
			if (t) try {
				return new Fl(e, this.interface, t);
			} catch (t) {
				return new Il(e, t);
			}
			return new Ol(e, l);
		});
	}
	async on(e, t) {
		let n = await cu(this, "on", e);
		return n.listeners.push({
			listener: t,
			once: !1
		}), n.start(), this;
	}
	async once(e, t) {
		let n = await cu(this, "once", e);
		return n.listeners.push({
			listener: t,
			once: !0
		}), n.start(), this;
	}
	async emit(e, ...t) {
		return await du(this, e, t, null);
	}
	async listenerCount(e) {
		if (e) {
			let t = await su(this, e);
			return t ? t.listeners.length : 0;
		}
		let { subs: t } = iu(this), n = 0;
		for (let { listeners: e } of t.values()) n += e.length;
		return n;
	}
	async listeners(e) {
		if (e) {
			let t = await su(this, e);
			return t ? t.listeners.map(({ listener: e }) => e) : [];
		}
		let { subs: t } = iu(this), n = [];
		for (let { listeners: e } of t.values()) n = n.concat(e.map(({ listener: e }) => e));
		return n;
	}
	async off(e, t) {
		let n = await su(this, e);
		if (!n) return this;
		if (t) {
			let e = n.listeners.map(({ listener: e }) => e).indexOf(t);
			e >= 0 && n.listeners.splice(e, 1);
		}
		return (t == null || n.listeners.length === 0) && (n.stop(), iu(this).subs.delete(n.tag)), this;
	}
	async removeAllListeners(e) {
		if (e) {
			let t = await su(this, e);
			if (!t) return this;
			t.stop(), iu(this).subs.delete(t.tag);
		} else {
			let { subs: e } = iu(this);
			for (let { tag: t, stop: n } of e.values()) n(), e.delete(t);
		}
		return this;
	}
	async addListener(e, t) {
		return await this.on(e, t);
	}
	async removeListener(e, t) {
		return await this.off(e, t);
	}
	static buildClass(t) {
		class n extends e {
			constructor(e, n = null) {
				super(e, t, n);
			}
		}
		return n;
	}
	static from(e, t, n) {
		return n ??= null, new this(e, t, n);
	}
};
function mu() {
	return pu;
}
var hu = class extends mu() {}, gu = BigInt(60);
function _u(e) {
	return e.match(/^ipfs:\/\/ipfs\//i) ? e = e.substring(12) : e.match(/^ipfs:\/\//i) ? e = e.substring(7) : d(!1, "unsupported IPFS format", "link", e), `https:/\/gateway.ipfs.io/ipfs/${e}`;
}
var vu = class {
	name;
	constructor(e) {
		a(this, { name: e });
	}
	connect(e) {
		return this;
	}
	supportsCoinType(e) {
		return !1;
	}
	async encodeAddress(e, t) {
		throw Error("unsupported coin");
	}
	async decodeAddress(e, t) {
		throw Error("unsupported coin");
	}
}, yu = /* @__PURE__ */ RegExp("^(ipfs)://(.*)$", "i"), bu = [
	/* @__PURE__ */ RegExp("^(https)://(.*)$", "i"),
	/* @__PURE__ */ RegExp("^(data):(.*)$", "i"),
	yu,
	/* @__PURE__ */ RegExp("^eip155:[0-9]+/(erc[0-9]+):(.*)$", "i")
];
function xu(e) {
	return e === gu || e >= 2147483648 && e <= 4294967295;
}
var Su = class e {
	provider;
	address;
	name;
	#e;
	#t;
	constructor(e, t, n, r) {
		a(this, {
			provider: e,
			address: t,
			name: n
		}), this.#e = r == null ? null : Promise.resolve(r), this.#t = new hu(t, [
			"function supportsInterface(bytes4) view returns (bool)",
			"function resolve(bytes, bytes) view returns (bytes)",
			"function addr(bytes32) view returns (address)",
			"function addr(bytes32, uint) view returns (bytes)",
			"function text(bytes32, string) view returns (string)",
			"function contenthash(bytes32) view returns (bytes)",
			"function name(bytes32) view returns (string)"
		], e);
	}
	async supportsWildcard() {
		return this.#e ??= (async () => {
			try {
				return await this.#t.supportsInterface("0x9061b923");
			} catch (e) {
				if (s(e, "CALL_EXCEPTION")) return !1;
				throw this.#e = null, e;
			}
		})(), await this.#e;
	}
	async #n(e, t) {
		t = (t || []).slice();
		let n = this.#t.interface;
		t.unshift($s(this.name));
		let r = null;
		await this.supportsWildcard() && (r = n.getFunction(e), u(r, "missing fragment", "UNKNOWN_ERROR", { info: { funcName: e } }), t = [ec(this.name, 255), n.encodeFunctionData(r, t)], e = "resolve(bytes,bytes)"), t.push({ enableCcipRead: !0 });
		try {
			let i = await this.#t[e](...t);
			return r ? n.decodeFunctionResult(r, i)[0] : i;
		} catch (e) {
			if (!s(e, "CALL_EXCEPTION")) throw e;
		}
		return null;
	}
	async getAddress(e) {
		let t = e == null ? gu : F(e);
		if (t === gu) try {
			let e = await this.#n("addr(bytes32)");
			return e == null || e === "0x0000000000000000000000000000000000000000" ? null : e;
		} catch (e) {
			if (s(e, "CALL_EXCEPTION")) return null;
			throw e;
		}
		if (xu(t)) {
			let e = await this.#n("addr(bytes32,uint)", [t]);
			return y(e, 20) ? K(e) : null;
		}
		if (t >= 0 && t < 2147483648) {
			let e = t + BigInt(2147483648), n = await this.#n("addr(bytes32,uint)", [e]);
			if (y(n, 20)) return K(n);
		}
		let n = null;
		for (let e of this.provider.plugins) if (e instanceof vu && t <= 2147483648 && e.supportsCoinType(Number(t))) {
			n = e;
			break;
		}
		if (n == null) return null;
		let r = await this.#n("addr(bytes32,uint)", [t]);
		if (r == null || r === "0x") return null;
		if (t < 2147483648) {
			let e = await n.decodeAddress(Number(t), r);
			if (e != null) return e;
		}
		u(!1, "invalid coin data", "UNSUPPORTED_OPERATION", {
			operation: `getAddress(${t})`,
			info: {
				coinType: t,
				data: r
			}
		});
	}
	async getText(e) {
		let t = await this.#n("text(bytes32,string)", [e]);
		return t == null || t === "0x" ? null : t;
	}
	async getContentHash() {
		let e = await this.#n("contenthash(bytes32)");
		if (e == null || e === "0x") return null;
		let t = e.match(/^0x(e3010170|e5010172)(([0-9a-f][0-9a-f])([0-9a-f][0-9a-f])([0-9a-f]*))$/);
		if (t) {
			let e = t[1] === "e3010170" ? "ipfs" : "ipns", n = parseInt(t[4], 16);
			if (t[5].length === n * 2) return `${e}:/\/${ie("0x" + t[2])}`;
		}
		let n = e.match(/^0xe40101fa011b20([0-9a-f]*)$/);
		if (n && n[1].length === 64) return `bzz:/\/${n[1]}`;
		u(!1, "invalid or unsupported content hash data", "UNSUPPORTED_OPERATION", {
			operation: "getContentHash()",
			info: { data: e }
		});
	}
	async getName() {
		return await this.#n("name(bytes32)");
	}
	async getAvatar() {
		return (await this._getAvatar()).url;
	}
	async _getAvatar() {
		let e = [{
			type: "name",
			value: this.name
		}];
		try {
			let t = await this.getText("avatar");
			if (t == null) return e.push({
				type: "!avatar",
				value: ""
			}), {
				url: null,
				linkage: e
			};
			e.push({
				type: "avatar",
				value: t
			});
			for (let n = 0; n < bu.length; n++) {
				let r = t.match(bu[n]);
				if (r == null) continue;
				let i = r[1].toLowerCase();
				switch (i) {
					case "https":
					case "data": return e.push({
						type: "url",
						value: t
					}), {
						linkage: e,
						url: t
					};
					case "ipfs": {
						let n = _u(t);
						return e.push({
							type: "ipfs",
							value: t
						}), e.push({
							type: "url",
							value: n
						}), {
							linkage: e,
							url: n
						};
					}
					case "erc721":
					case "erc1155": {
						let n = i === "erc721" ? "tokenURI(uint256)" : "uri(uint256)";
						e.push({
							type: i,
							value: t
						});
						let a = await this.getAddress();
						if (a == null) return e.push({
							type: "!owner",
							value: ""
						}), {
							url: null,
							linkage: e
						};
						let o = (r[2] || "").split("/");
						if (o.length !== 2) return e.push({
							type: `!${i}caip`,
							value: r[2] || ""
						}), {
							url: null,
							linkage: e
						};
						let s = o[1], c = new hu(o[0], [
							"function tokenURI(uint) view returns (string)",
							"function ownerOf(uint) view returns (address)",
							"function uri(uint) view returns (string)",
							"function balanceOf(address, uint256) view returns (uint)"
						], this.provider);
						if (i === "erc721") {
							let t = await c.ownerOf(s);
							if (a !== t) return e.push({
								type: "!owner",
								value: t
							}), {
								url: null,
								linkage: e
							};
							e.push({
								type: "owner",
								value: t
							});
						} else if (i === "erc1155") {
							let t = await c.balanceOf(a, s);
							if (!t) return e.push({
								type: "!balance",
								value: "0"
							}), {
								url: null,
								linkage: e
							};
							e.push({
								type: "balance",
								value: t.toString()
							});
						}
						let l = await c[n](s);
						if (l == null || l === "0x") return e.push({
							type: "!metadata-url",
							value: ""
						}), {
							url: null,
							linkage: e
						};
						e.push({
							type: "metadata-url-base",
							value: l
						}), i === "erc1155" && (l = l.replace("{id}", te(s, 32).substring(2)), e.push({
							type: "metadata-url-expanded",
							value: l
						})), l.match(/^ipfs:/i) && (l = _u(l)), e.push({
							type: "metadata-url",
							value: l
						});
						let u = {}, d = await new Oe(l).send();
						d.assertOk();
						try {
							u = d.bodyJson;
						} catch {
							try {
								e.push({
									type: "!metadata",
									value: d.bodyText
								});
							} catch {
								let t = d.body;
								return t && e.push({
									type: "!metadata",
									value: S(t)
								}), {
									url: null,
									linkage: e
								};
							}
							return {
								url: null,
								linkage: e
							};
						}
						if (!u) return e.push({
							type: "!metadata",
							value: ""
						}), {
							url: null,
							linkage: e
						};
						e.push({
							type: "metadata",
							value: JSON.stringify(u)
						});
						let f = u.image;
						if (typeof f != "string") return e.push({
							type: "!imageUrl",
							value: ""
						}), {
							url: null,
							linkage: e
						};
						if (!f.match(/^(https:\/\/|data:)/i)) {
							if (f.match(yu) == null) return e.push({
								type: "!imageUrl-ipfs",
								value: f
							}), {
								url: null,
								linkage: e
							};
							e.push({
								type: "imageUrl-ipfs",
								value: f
							}), f = _u(f);
						}
						return e.push({
							type: "url",
							value: f
						}), {
							linkage: e,
							url: f
						};
					}
				}
			}
		} catch {}
		return {
			linkage: e,
			url: null
		};
	}
	static async getEnsAddress(e) {
		let t = await e.getNetwork(), n = t.getPlugin("org.ethers.plugins.network.Ens");
		return u(n, "network does not support ENS", "UNSUPPORTED_OPERATION", {
			operation: "getEnsAddress",
			info: { network: t }
		}), n.address;
	}
	static async getUniversalResolverAddress(e) {
		let t = (await e.getNetwork()).getPlugin("org.ethers.plugins.network.Ens");
		return t && t.universalResolver ? t.universalResolver : null;
	}
	static async #r(t, n) {
		let r = await e.getEnsAddress(t);
		try {
			let e = await new hu(r, ["function resolver(bytes32) view returns (address)"], t).resolver($s(n), { enableCcipRead: !0 });
			return e === "0x0000000000000000000000000000000000000000" ? null : e;
		} catch (e) {
			throw e;
		}
	}
	static async lookupAddress(t, n, r) {
		let i = r == null ? gu : F(r);
		xu(i) && (n = K(n));
		let a = await Cu(t);
		if (a) try {
			let e = (await a.reverse(n, i, { enableCcipRead: !0 })).primary;
			return Qs(e) ? e : null;
		} catch (e) {
			if (s(e, "CALL_EXCEPTION") && e.reason === "ResolverNotFound(bytes)") return null;
			throw e;
		}
		u(i === gu, "lookupAddress coinType requires ENS Universal Resolver", "UNSUPPORTED_OPERATION", { operation: "lookupAddress" });
		try {
			let r = await e.fromName(t, `${n.toLowerCase().substring(2)}.addr.reverse`);
			if (!r) return null;
			let i = await r.getName();
			return i == null || !Qs(i) || await t.resolveName(i) !== n ? null : i;
		} catch (e) {
			if (s(e, "BAD_DATA") && e.value === "0x" || s(e, "CALL_EXCEPTION")) return null;
			throw e;
		}
	}
	static async fromName(t, n) {
		let r = await Cu(t);
		if (r) {
			let i;
			try {
				i = ec(Zs(n), 255);
			} catch {
				return null;
			}
			let a = await r.requireResolver(i);
			return new e(t, a.resolver, n, a.extended);
		}
		let i = n;
		for (;;) {
			if (i === "" || i === "." || n !== "eth" && i === "eth") return null;
			let r = await e.#r(t, i);
			if (r != null) {
				let a = new e(t, r, n);
				return i !== n && !await a.supportsWildcard() ? null : a;
			}
			i = i.split(".").slice(1).join(".");
		}
	}
};
async function Cu(e) {
	let t = await Su.getUniversalResolverAddress(e);
	return t ? new hu(t, [
		"function requireResolver(bytes) view returns ((bytes name, uint256 offset, bytes32 node, address resolver, bool extended))",
		"function findResolver(bytes) view returns (address resolver, bytes32 node, uint offset)",
		"function resolve(bytes name, bytes data) view returns (bytes result, address resolver)",
		"function reverse(bytes name, uint coinType) view returns (string primary, address resolver, address reverseResolver)",
		"error ResolverNotFound(bytes name)",
		"error ResolverNotContract(bytes name, address resolver)",
		"error ReverseAddressMismatch(string primary, bytes primaryAddress)",
		"error HttpError(uint16 statusCode, string statusMessage)"
	], e) : null;
}
//#endregion
//#region node_modules/ethers/lib.esm/providers/format.js
var wu = BigInt(0);
function Z(e, t) {
	return (function(n) {
		return n == null ? t : e(n);
	});
}
function Tu(e, t) {
	return ((n) => {
		if (t && n == null) return null;
		if (!Array.isArray(n)) throw Error("not an array");
		return n.map((t) => e(t));
	});
}
function Eu(e, t) {
	return ((n) => {
		let r = {};
		for (let i in e) {
			let a = i;
			if (t && i in t && !(a in n)) {
				for (let e of t[i]) if (e in n) {
					a = e;
					break;
				}
			}
			try {
				let t = e[i](n[a]);
				t !== void 0 && (r[i] = t);
			} catch (e) {
				u(!1, `invalid value for value.${i} (${e instanceof Error ? e.message : "not-an-error"})`, "BAD_DATA", { value: n });
			}
		}
		return r;
	});
}
function Du(e) {
	switch (e) {
		case !0:
		case "true": return !0;
		case !1:
		case "false": return !1;
	}
	d(!1, `invalid boolean; ${JSON.stringify(e)}`, "value", e);
}
function Ou(e) {
	return d(y(e, !0), "invalid data", "value", e), e;
}
function ku(e) {
	return d(y(e, 32), "invalid hash", "value", e), e;
}
var Au = Eu({
	address: K,
	blockHash: ku,
	blockNumber: z,
	data: Ou,
	index: z,
	removed: Z(Du, !1),
	topics: Tu(ku),
	transactionHash: ku,
	transactionIndex: z
}, { index: ["logIndex"] });
function ju(e) {
	return Au(e);
}
var Mu = Eu({
	hash: Z(ku),
	parentHash: ku,
	parentBeaconBlockRoot: Z(ku, null),
	number: z,
	timestamp: z,
	nonce: Z(Ou),
	difficulty: F,
	gasLimit: F,
	gasUsed: F,
	stateRoot: Z(ku, null),
	receiptsRoot: Z(ku, null),
	transactionsRoot: Z(ku, null),
	blobGasUsed: Z(F, null),
	excessBlobGas: Z(F, null),
	miner: Z(K),
	prevRandao: Z(ku, null),
	extraData: Ou,
	baseFeePerGas: Z(F)
}, { prevRandao: ["mixHash"] });
function Nu(e) {
	let t = Mu(e);
	return t.transactions = e.transactions.map((e) => typeof e == "string" ? e : Ru(e)), t;
}
var Pu = Eu({
	transactionIndex: z,
	blockNumber: z,
	transactionHash: ku,
	address: K,
	topics: Tu(ku),
	data: Ou,
	index: z,
	blockHash: ku
}, { index: ["logIndex"] });
function Fu(e) {
	return Pu(e);
}
var Iu = Eu({
	to: Z(K, null),
	from: Z(K, null),
	contractAddress: Z(K, null),
	index: z,
	root: Z(S),
	gasUsed: F,
	blobGasUsed: Z(F, null),
	logsBloom: Z(Ou),
	blockHash: ku,
	hash: ku,
	logs: Tu(Fu),
	blockNumber: z,
	cumulativeGasUsed: F,
	effectiveGasPrice: Z(F),
	blobGasPrice: Z(F, null),
	status: Z(z),
	type: Z(z, 0)
}, {
	effectiveGasPrice: ["gasPrice"],
	hash: ["transactionHash"],
	index: ["transactionIndex"]
});
function Lu(e) {
	return Iu(e);
}
function Ru(e) {
	e.to && F(e.to) === wu && (e.to = "0x0000000000000000000000000000000000000000");
	let t = Eu({
		hash: ku,
		index: Z(z, void 0),
		type: (e) => e === "0x" || e == null ? 0 : z(e),
		accessList: Z(Fa, null),
		blobVersionedHashes: Z(Tu(ku, !0), null),
		authorizationList: Z(Tu((e) => {
			let t;
			if (e.signature) t = e.signature;
			else {
				let n = e.yParity;
				n === "0x1b" ? n = 0 : n === "0x1c" && (n = 1), t = Object.assign({}, e, { yParity: n });
			}
			return {
				address: K(e.address),
				chainId: F(e.chainId),
				nonce: F(e.nonce),
				signature: ea.from(t)
			};
		}, !1), null),
		blockHash: Z(ku, null),
		blockNumber: Z(z, null),
		transactionIndex: Z(z, null),
		from: K,
		gasPrice: Z(F),
		maxPriorityFeePerGas: Z(F),
		maxFeePerGas: Z(F),
		maxFeePerBlobGas: Z(F, null),
		gasLimit: F,
		to: Z(K, null),
		value: F,
		nonce: z,
		data: Ou,
		creates: Z(K, null),
		chainId: Z(F, null)
	}, {
		data: ["input"],
		gasLimit: ["gas"],
		index: ["transactionIndex"]
	})(e);
	if (t.to == null && t.creates == null && (t.creates = ua(t)), (e.type === 1 || e.type === 2) && e.accessList == null && (t.accessList = []), t.signature = e.signature ? ea.from(e.signature) : ea.from(e), t.chainId == null) {
		let e = t.signature.legacyChainId;
		e != null && (t.chainId = e);
	}
	return t.blockHash && F(t.blockHash) === wu && (t.blockHash = null), t;
}
//#endregion
//#region node_modules/ethers/lib.esm/providers/plugins-network.js
var zu = "0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e", Bu = Symbol.for("nodejs.util.inspect.custom"), Vu = class e {
	name;
	constructor(e) {
		a(this, { name: e });
	}
	[Bu]() {
		return this.toString();
	}
	toString() {
		return `${this.name} { }`;
	}
	clone() {
		return new e(this.name);
	}
}, Hu = class e extends Vu {
	effectiveBlock;
	txBase;
	txCreate;
	txDataZero;
	txDataNonzero;
	txAccessListStorageKey;
	txAccessListAddress;
	constructor(e, t) {
		e ??= 0, super(`org.ethers.network.plugins.GasCost#${e || 0}`);
		let n = { effectiveBlock: e };
		function r(e, r) {
			let i = (t || {})[e];
			i ??= r, d(typeof i == "number", `invalud value for ${e}`, "costs", t), n[e] = i;
		}
		r("txBase", 21e3), r("txCreate", 32e3), r("txDataZero", 4), r("txDataNonzero", 16), r("txAccessListStorageKey", 1900), r("txAccessListAddress", 2400), a(this, n);
	}
	toString() {
		return `${this.name} { txBase: ${this.txBase}, txCreate: ${this.txCreate}, txDataZero: ${this.txDataZero}, txAccessListStorageKey: ${this.txAccessListStorageKey}, txAccessListAddress: ${this.txAccessListAddress} }`;
	}
	clone() {
		return new e(this.effectiveBlock, this);
	}
}, Uu = class e extends Vu {
	address;
	targetNetwork;
	universalResolver;
	constructor(e, t, n) {
		super("org.ethers.plugins.network.Ens"), a(this, {
			address: e || zu,
			targetNetwork: t ?? 1,
			universalResolver: n
		});
	}
	toString() {
		return `${this.name} { address: ${this.address}, targetNetwork: ${this.targetNetwork}, universalResolver: ${this.universalResolver} }`;
	}
	clone() {
		return new e(this.address, this.targetNetwork, this.universalResolver);
	}
}, Wu = class extends Vu {
	#e;
	#t;
	get url() {
		return this.#e;
	}
	get processFunc() {
		return this.#t;
	}
	constructor(e, t) {
		super("org.ethers.plugins.network.FetchUrlFeeDataPlugin"), this.#e = e, this.#t = t;
	}
	toString() {
		return `${this.name} { url: ${this.url} }`;
	}
	clone() {
		return this;
	}
}, Gu = Symbol.for("nodejs.util.inspect.custom"), Ku = /* @__PURE__ */ new Map(), qu = class e {
	#e;
	#t;
	#n;
	constructor(e, t) {
		this.#e = e, this.#t = F(t), this.#n = /* @__PURE__ */ new Map();
	}
	[Gu]() {
		return this.toString();
	}
	toString() {
		let e = [];
		for (let t of this.#n.values()) e.push(t.toString());
		return `Network { name: ${this.name}, chainId: ${this.chainId}, plugins: [ ${e.join(", ")} ] }`;
	}
	toJSON() {
		return {
			name: this.name,
			chainId: String(this.chainId)
		};
	}
	get name() {
		return this.#e;
	}
	set name(e) {
		this.#e = e;
	}
	get chainId() {
		return this.#t;
	}
	set chainId(e) {
		this.#t = F(e, "chainId");
	}
	matches(e) {
		if (e == null) return !1;
		if (typeof e == "string") {
			try {
				return this.chainId === F(e);
			} catch {}
			return this.name === e;
		}
		if (typeof e == "number" || typeof e == "bigint") {
			try {
				return this.chainId === F(e);
			} catch {}
			return !1;
		}
		if (typeof e == "object") {
			if (e.chainId != null) {
				try {
					return this.chainId === F(e.chainId);
				} catch {}
				return !1;
			}
			return e.name != null && this.name === e.name;
		}
		return !1;
	}
	get plugins() {
		return Array.from(this.#n.values());
	}
	attachPlugin(e) {
		if (this.#n.get(e.name)) throw Error(`cannot replace existing plugin: ${e.name} `);
		return this.#n.set(e.name, e.clone()), this;
	}
	getPlugin(e) {
		return this.#n.get(e) || null;
	}
	getPlugins(e) {
		return this.plugins.filter((t) => t.name.split("#")[0] === e);
	}
	clone() {
		let t = new e(this.name, this.chainId);
		return this.plugins.forEach((e) => {
			t.attachPlugin(e.clone());
		}), t;
	}
	computeIntrinsicGas(e) {
		let t = this.getPlugin("org.ethers.plugins.network.GasCost") || new Hu(), n = t.txBase;
		if (e.to ?? (n += t.txCreate), e.data) for (let r = 2; r < e.data.length; r += 2) e.data.substring(r, r + 2) === "00" ? n += t.txDataZero : n += t.txDataNonzero;
		if (e.accessList) {
			let r = Fa(e.accessList);
			for (let e in r) n += t.txAccessListAddress + t.txAccessListStorageKey * r[e].storageKeys.length;
		}
		return n;
	}
	static from(t) {
		if (Zu(), t == null) return e.from("mainnet");
		if (typeof t == "number" && (t = BigInt(t)), typeof t == "string" || typeof t == "bigint") {
			let n = Ku.get(t);
			if (n) return n();
			if (typeof t == "bigint") return new e("unknown", t);
			d(!1, "unknown network", "network", t);
		}
		if (typeof t.clone == "function") return t.clone();
		if (typeof t == "object") {
			d(typeof t.name == "string" && typeof t.chainId == "number", "invalid network object name or chainId", "network", t);
			let n = new e(t.name, t.chainId), r = t;
			return (r.ensAddress || r.ensNetwork != null || r.ensUniversalResolver) && n.attachPlugin(new Uu(r.ensAddress, r.ensNetwork, r.ensUniversalResolver)), n;
		}
		d(!1, "invalid network", "network", t);
	}
	static register(e, t) {
		typeof e == "number" && (e = BigInt(e));
		let n = Ku.get(e);
		n && d(!1, `conflicting network for ${JSON.stringify(n.name)}`, "nameOrChainId", e), Ku.set(e, t);
	}
};
function Ju(e, t) {
	let n = String(e);
	if (!n.match(/^[0-9.]+$/)) throw Error(`invalid gwei value: ${e}`);
	let r = n.split(".");
	if (r.length === 1 && r.push(""), r.length !== 2) throw Error(`invalid gwei value: ${e}`);
	for (; r[1].length < t;) r[1] += "0";
	if (r[1].length > 9) {
		let e = BigInt(r[1].substring(0, 9));
		r[1].substring(9).match(/^0+$/) || e++, r[1] = e.toString();
	}
	return BigInt(r[0] + r[1]);
}
function Yu(e) {
	return new Wu(e, async (e, t, n) => {
		n.setHeader("User-Agent", "ethers");
		let r;
		try {
			let [t, i] = await Promise.all([n.send(), e()]);
			r = t;
			let a = r.bodyJson.standard;
			return {
				gasPrice: i.gasPrice,
				maxFeePerGas: Ju(a.maxFee, 9),
				maxPriorityFeePerGas: Ju(a.maxPriorityFee, 9)
			};
		} catch (e) {
			u(!1, `error encountered with polygon gas station (${JSON.stringify(n.url)})`, "SERVER_ERROR", {
				request: n,
				response: r,
				error: e
			});
		}
	});
}
var Xu = !1;
function Zu() {
	if (Xu) return;
	Xu = !0;
	function e(e, t, n) {
		let r = function() {
			let r = new qu(e, t);
			return n.ensNetwork != null && r.attachPlugin(new Uu(null, n.ensNetwork, n.ensUniversalResolver)), r.attachPlugin(new Hu()), (n.plugins || []).forEach((e) => {
				r.attachPlugin(e);
			}), r;
		};
		qu.register(e, r), qu.register(t, r), n.altNames && n.altNames.forEach((e) => {
			qu.register(e, r);
		});
	}
	let t = "0xeEeEEEeE14D718C2B47D9923Deab1335E144EeEe";
	e("mainnet", 1, {
		ensUniversalResolver: t,
		ensNetwork: 1,
		altNames: ["homestead"]
	}), e("ropsten", 3, { ensNetwork: 3 }), e("rinkeby", 4, { ensNetwork: 4 }), e("goerli", 5, { ensNetwork: 5 }), e("kovan", 42, { ensNetwork: 42 }), e("sepolia", 11155111, {
		ensUniversalResolver: t,
		ensNetwork: 11155111
	}), e("holesky", 17e3, { ensNetwork: 17e3 }), e("classic", 61, {}), e("classicKotti", 6, {}), e("arbitrum", 42161, { ensNetwork: 1 }), e("arbitrum-goerli", 421613, {}), e("arbitrum-sepolia", 421614, {}), e("base", 8453, { ensNetwork: 1 }), e("base-goerli", 84531, {}), e("base-sepolia", 84532, {}), e("bnb", 56, { ensNetwork: 1 }), e("bnbt", 97, {}), e("filecoin", 314, {}), e("filecoin-calibration", 314159, {}), e("linea", 59144, { ensNetwork: 1 }), e("linea-goerli", 59140, {}), e("linea-sepolia", 59141, {}), e("matic", 137, {
		ensNetwork: 1,
		plugins: [Yu("https://gasstation.polygon.technology/v2")]
	}), e("matic-amoy", 80002, {}), e("matic-mumbai", 80001, {
		altNames: ["maticMumbai", "maticmum"],
		plugins: [Yu("https://gasstation-testnet.polygon.technology/v2")]
	}), e("optimism", 10, {
		ensNetwork: 1,
		plugins: []
	}), e("optimism-goerli", 420, {}), e("optimism-sepolia", 11155420, {}), e("xdai", 100, { ensNetwork: 1 });
}
//#endregion
//#region node_modules/ethers/lib.esm/providers/subscriber-polling.js
function Qu(e) {
	return JSON.parse(JSON.stringify(e));
}
var $u = class {
	#e;
	#t;
	#n;
	#r;
	constructor(e) {
		this.#e = e, this.#t = null, this.#n = 4e3, this.#r = -2;
	}
	get pollingInterval() {
		return this.#n;
	}
	set pollingInterval(e) {
		this.#n = e;
	}
	async #i() {
		try {
			let e = await this.#e.getBlockNumber();
			if (this.#r === -2) {
				this.#r = e;
				return;
			}
			if (e !== this.#r) {
				for (let t = this.#r + 1; t <= e; t++) {
					if (this.#t == null) return;
					await this.#e.emit("block", t);
				}
				this.#r = e;
			}
		} catch {}
		this.#t != null && (this.#t = this.#e._setTimeout(this.#i.bind(this), this.#n));
	}
	start() {
		this.#t || (this.#t = this.#e._setTimeout(this.#i.bind(this), this.#n), this.#i());
	}
	stop() {
		this.#t &&= (this.#e._clearTimeout(this.#t), null);
	}
	pause(e) {
		this.stop(), e && (this.#r = -2);
	}
	resume() {
		this.start();
	}
}, ed = class {
	#e;
	#t;
	#n;
	constructor(e) {
		this.#e = e, this.#n = !1, this.#t = (e) => {
			this._poll(e, this.#e);
		};
	}
	async _poll(e, t) {
		throw Error("sub-classes must override this");
	}
	start() {
		this.#n || (this.#n = !0, this.#t(-2), this.#e.on("block", this.#t));
	}
	stop() {
		this.#n && (this.#n = !1, this.#e.off("block", this.#t));
	}
	pause(e) {
		this.stop();
	}
	resume() {
		this.start();
	}
}, td = class extends ed {
	#e;
	#t;
	constructor(e, t) {
		super(e), this.#e = t, this.#t = -2;
	}
	pause(e) {
		e && (this.#t = -2), super.pause(e);
	}
	async _poll(e, t) {
		let n = await t.getBlock(this.#e);
		n != null && (this.#t === -2 ? this.#t = n.number : n.number > this.#t && (t.emit(this.#e, n.number), this.#t = n.number));
	}
}, nd = class extends ed {
	#e;
	constructor(e, t) {
		super(e), this.#e = Qu(t);
	}
	async _poll(e, t) {
		throw Error("@TODO");
	}
}, rd = class extends ed {
	#e;
	constructor(e, t) {
		super(e), this.#e = t;
	}
	async _poll(e, t) {
		let n = await t.getTransactionReceipt(this.#e);
		n && t.emit(this.#e, n);
	}
}, id = class {
	#e;
	#t;
	#n;
	#r;
	#i;
	constructor(e, t) {
		this.#e = e, this.#t = Qu(t), this.#n = this.#a.bind(this), this.#r = !1, this.#i = -2;
	}
	async #a(e) {
		if (this.#i === -2) return;
		let t = Qu(this.#t);
		t.fromBlock = this.#i + 1, t.toBlock = e;
		let n = await this.#e.getLogs(t);
		if (n.length === 0) {
			this.#i < e - 60 && (this.#i = e - 60);
			return;
		}
		for (let e of n) this.#e.emit(this.#t, e), this.#i = e.blockNumber;
	}
	start() {
		this.#r || (this.#r = !0, this.#i === -2 && this.#e.getBlockNumber().then((e) => {
			this.#i = e;
		}), this.#e.on("block", this.#n));
	}
	stop() {
		this.#r && (this.#r = !1, this.#e.off("block", this.#n));
	}
	pause(e) {
		this.stop(), e && (this.#i = -2);
	}
	resume() {
		this.start();
	}
}, ad = BigInt(2), od = 10;
function sd(e) {
	return new Promise((t) => {
		setTimeout(t, e);
	});
}
function cd(e) {
	return e && typeof e.then == "function";
}
function ld(e, t) {
	return e + ":" + JSON.stringify(t, (e, t) => {
		if (t == null) return "null";
		if (typeof t == "bigint") return `bigint:${t.toString()}`;
		if (typeof t == "string") return t.toLowerCase();
		if (typeof t == "object" && !Array.isArray(t)) {
			let e = Object.keys(t);
			return e.sort(), e.reduce((e, n) => (e[n] = t[n], e), {});
		}
		return t;
	});
}
var ud = class {
	name;
	constructor(e) {
		a(this, { name: e });
	}
	start() {}
	stop() {}
	pause(e) {}
	resume() {}
};
function dd(e) {
	return JSON.parse(JSON.stringify(e));
}
function fd(e) {
	return e = Array.from(new Set(e).values()), e.sort(), e;
}
async function pd(e, t) {
	if (e == null) throw Error("invalid event");
	if (Array.isArray(e) && (e = { topics: e }), typeof e == "string") switch (e) {
		case "block":
		case "debug":
		case "error":
		case "finalized":
		case "network":
		case "pending":
		case "safe": return {
			type: e,
			tag: e
		};
	}
	if (y(e, 32)) {
		let t = e.toLowerCase();
		return {
			type: "transaction",
			tag: ld("tx", { hash: t }),
			hash: t
		};
	}
	if (e.orphan) {
		let t = e;
		return {
			type: "orphan",
			tag: ld("orphan", t),
			filter: dd(t)
		};
	}
	if (e.address || e.topics) {
		let n = e, r = { topics: (n.topics || []).map((e) => e == null ? null : Array.isArray(e) ? fd(e.map((e) => e.toLowerCase())) : e.toLowerCase()) };
		if (n.address) {
			let e = [], i = [], a = (n) => {
				y(n) ? e.push(n) : i.push((async () => {
					e.push(await pa(n, t));
				})());
			};
			Array.isArray(n.address) ? n.address.forEach(a) : a(n.address), i.length && await Promise.all(i), r.address = fd(e.map((e) => e.toLowerCase()));
		}
		return {
			filter: r,
			tag: ld("event", r),
			type: "event"
		};
	}
	d(!1, "unknown ProviderEvent", "event", e);
}
function md() {
	return (/* @__PURE__ */ new Date()).getTime();
}
var hd = {
	cacheTimeout: 250,
	pollingInterval: 4e3
}, gd = class {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	#o;
	#s;
	#c;
	#l;
	#u;
	#d;
	#f;
	#p;
	constructor(e, t) {
		if (this.#p = Object.assign({}, hd, t || {}), e === "any") this.#a = !0, this.#i = null;
		else if (e) {
			let t = qu.from(e);
			this.#a = !1, this.#i = Promise.resolve(t), setTimeout(() => {
				this.emit("network", t, null);
			}, 0);
		} else this.#a = !1, this.#i = null;
		this.#s = -1, this.#o = /* @__PURE__ */ new Map(), this.#e = /* @__PURE__ */ new Map(), this.#t = /* @__PURE__ */ new Map(), this.#n = null, this.#r = !1, this.#c = 1, this.#l = /* @__PURE__ */ new Map(), this.#u = !1, this.#d = 0, this.#f = [];
	}
	get _requestRate() {
		let e = this.#d;
		return e == 0 ? null : e;
	}
	set _requestRate(e) {
		(e == null || e < 0) && (e = 0), this.#d = z(e);
	}
	get pollingInterval() {
		return this.#p.pollingInterval;
	}
	get provider() {
		return this;
	}
	get plugins() {
		return Array.from(this.#t.values());
	}
	attachPlugin(e) {
		if (this.#t.get(e.name)) throw Error(`cannot replace existing plugin: ${e.name} `);
		return this.#t.set(e.name, e.connect(this)), this;
	}
	getPlugin(e) {
		return this.#t.get(e) || null;
	}
	get disableCcipRead() {
		return this.#u;
	}
	set disableCcipRead(e) {
		this.#u = !!e;
	}
	#m() {
		let e = this.#d;
		if (e === 0) return 0;
		let t = this.#f, n = md();
		t.push(n);
		let r = n - 1e3;
		for (; t.length && t[0] < r;) t.shift();
		return t.length < e ? 0 : t[0] + 1e3 - n;
	}
	async #h(e) {
		let t = this.#p.cacheTimeout;
		if (t < 0) {
			let t = this.#m();
			return t && await sd(t), await this._perform(e);
		}
		let n = ld(e.method, e), r = this.#o.get(n);
		if (!r) {
			let i = this.#m();
			i && await sd(i), r = this._perform(e), this.#o.set(n, r), setTimeout(() => {
				this.#o.get(n) === r && this.#o.delete(n);
			}, t);
		}
		return await r;
	}
	async ccipReadFetch(e, t, n) {
		if (this.disableCcipRead || n.length === 0 || e.to == null) return null;
		let r = e.to.toLowerCase(), i = t.toLowerCase(), a = [];
		for (let t = 0; t < n.length; t++) {
			let o = n[t], s = new Oe(o.replace("{sender}", r).replace("{data}", i));
			o.indexOf("{data}") === -1 && (s.body = {
				data: i,
				sender: r
			}), this.emit("debug", {
				action: "sendCcipReadFetchRequest",
				request: s,
				index: t,
				urls: n
			});
			let c = "unknown error", l;
			try {
				l = await s.send();
			} catch (e) {
				a.push(e.message), this.emit("debug", {
					action: "receiveCcipReadFetchError",
					request: s,
					result: { error: e }
				});
				continue;
			}
			try {
				let e = l.bodyJson;
				if (e.data) return this.emit("debug", {
					action: "receiveCcipReadFetchResult",
					request: s,
					result: e
				}), e.data;
				e.message && (c = e.message), this.emit("debug", {
					action: "receiveCcipReadFetchError",
					request: s,
					result: e
				});
			} catch {}
			u(l.statusCode < 400 || l.statusCode >= 500, `response not found during CCIP fetch: ${c}`, "OFFCHAIN_FAULT", {
				reason: "404_MISSING_RESOURCE",
				transaction: e,
				info: {
					url: o,
					errorMessage: c
				}
			}), a.push(c);
		}
		u(!1, `error encountered during CCIP fetch: ${a.map((e) => JSON.stringify(e)).join(", ")}`, "OFFCHAIN_FAULT", {
			reason: "500_SERVER_ERROR",
			transaction: e,
			info: {
				urls: n,
				errorMessages: a
			}
		});
	}
	_wrapBlock(e, t) {
		return new Dl(Nu(e), this);
	}
	_wrapLog(e, t) {
		return new Ol(ju(e), this);
	}
	_wrapTransactionReceipt(e, t) {
		return new kl(Lu(e), this);
	}
	_wrapTransactionResponse(e, t) {
		return new Al(Ru(e), this);
	}
	_detectNetwork() {
		u(!1, "sub-classes must implement this", "UNSUPPORTED_OPERATION", { operation: "_detectNetwork" });
	}
	async _perform(e) {
		u(!1, `unsupported method: ${e.method}`, "UNSUPPORTED_OPERATION", {
			operation: e.method,
			info: e
		});
	}
	async getBlockNumber() {
		let e = z(await this.#h({ method: "getBlockNumber" }), "%response");
		return this.#s >= 0 && (this.#s = e), e;
	}
	_getAddress(e) {
		return pa(e, this);
	}
	_getBlockTag(e) {
		if (e == null) return "latest";
		switch (e) {
			case "earliest": return "0x0";
			case "finalized":
			case "latest":
			case "pending":
			case "safe": return e;
		}
		if (y(e)) return y(e, 32) ? e : V(e);
		if (typeof e == "bigint" && (e = z(e, "blockTag")), typeof e == "number") return e >= 0 ? V(e) : this.#s >= 0 ? V(this.#s + e) : this.getBlockNumber().then((t) => V(t + e));
		d(!1, "invalid blockTag", "blockTag", e);
	}
	_getFilter(e) {
		let t = (e.topics || []).map((e) => e == null ? null : Array.isArray(e) ? fd(e.map((e) => e.toLowerCase())) : e.toLowerCase()), n = "blockHash" in e ? e.blockHash : void 0, r = (e, r, i) => {
			let a;
			switch (e.length) {
				case 0: break;
				case 1:
					a = e[0];
					break;
				default: e.sort(), a = e;
			}
			if (n && (r != null || i != null)) throw Error("invalid filter");
			let o = {};
			return a && (o.address = a), t.length && (o.topics = t), r && (o.fromBlock = r), i && (o.toBlock = i), n && (o.blockHash = n), o;
		}, i = [];
		if (e.address) {
			if (Array.isArray(e.address)) for (let t of e.address) i.push(this._getAddress(t));
			else i.push(this._getAddress(e.address));
		}
		let a;
		"fromBlock" in e && (a = this._getBlockTag(e.fromBlock));
		let o;
		return "toBlock" in e && (o = this._getBlockTag(e.toBlock)), i.filter((e) => typeof e != "string").length || a != null && typeof a != "string" || o != null && typeof o != "string" ? Promise.all([
			Promise.all(i),
			a,
			o
		]).then((e) => r(e[0], e[1], e[2])) : r(i, a, o);
	}
	_getTransactionRequest(e) {
		let t = El(e), n = [];
		if (["to", "from"].forEach((e) => {
			if (t[e] == null) return;
			let r = pa(t[e], this);
			cd(r) ? n.push((async function() {
				t[e] = await r;
			})()) : t[e] = r;
		}), t.blockTag != null) {
			let e = this._getBlockTag(t.blockTag);
			cd(e) ? n.push((async function() {
				t.blockTag = await e;
			})()) : t.blockTag = e;
		}
		return n.length ? (async function() {
			return await Promise.all(n), t;
		})() : t;
	}
	async getNetwork() {
		if (this.#i == null) {
			let e = (async () => {
				try {
					let e = await this._detectNetwork();
					return this.emit("network", e, null), e;
				} catch (t) {
					throw this.#i === e && (this.#i = null), t;
				}
			})();
			return this.#i = e, (await e).clone();
		}
		let e = this.#i, [t, n] = await Promise.all([e, this._detectNetwork()]);
		return t.chainId !== n.chainId && (this.#a ? (this.emit("network", n, t), this.#i === e && (this.#i = Promise.resolve(n))) : u(!1, `network changed: ${t.chainId} => ${n.chainId} `, "NETWORK_ERROR", { event: "changed" })), t.clone();
	}
	async getFeeData() {
		let e = await this.getNetwork(), t = async () => {
			let { _block: t, gasPrice: n, priorityFee: r } = await i({
				_block: this.#y("latest", !1),
				gasPrice: (async () => {
					try {
						return F(await this.#h({ method: "getGasPrice" }), "%response");
					} catch {}
					return null;
				})(),
				priorityFee: (async () => {
					try {
						return F(await this.#h({ method: "getPriorityFee" }), "%response");
					} catch {}
					return null;
				})()
			}), a = null, o = null, s = this._wrapBlock(t, e);
			return s && s.baseFeePerGas && (o = r ?? BigInt("1000000000"), a = s.baseFeePerGas * ad + o), new Tl(n, a, o);
		}, n = e.getPlugin("org.ethers.plugins.network.FetchUrlFeeDataPlugin");
		if (n) {
			let e = new Oe(n.url), r = await n.processFunc(t, this, e);
			return new Tl(r.gasPrice, r.maxFeePerGas, r.maxPriorityFeePerGas);
		}
		return await t();
	}
	async estimateGas(e) {
		let t = this._getTransactionRequest(e);
		return cd(t) && (t = await t), F(await this.#h({
			method: "estimateGas",
			transaction: t
		}), "%response");
	}
	async #g(e, t, n) {
		u(n < od, "CCIP read exceeded maximum redirections", "OFFCHAIN_FAULT", {
			reason: "TOO_MANY_REDIRECTS",
			transaction: Object.assign({}, e, {
				blockTag: t,
				enableCcipRead: !0
			})
		});
		let r = El(e);
		try {
			let e = this.#m();
			return e && await sd(e), S(await this._perform({
				method: "call",
				transaction: r,
				blockTag: t
			}));
		} catch (e) {
			if (!this.disableCcipRead && c(e) && e.data && n >= 0 && t === "latest" && r.to != null && T(e.data, 0, 4) === "0x556f1830") {
				let i = e.data, a = await pa(r.to, this), o;
				try {
					o = wd(T(e.data, 4));
				} catch (e) {
					u(!1, e.message, "OFFCHAIN_FAULT", {
						reason: "BAD_DATA",
						transaction: r,
						info: { data: i }
					});
				}
				u(o.sender.toLowerCase() === a.toLowerCase(), "CCIP Read sender mismatch", "CALL_EXCEPTION", {
					action: "call",
					data: i,
					reason: "OffchainLookup",
					transaction: r,
					invocation: null,
					revert: {
						signature: "OffchainLookup(address,string[],bytes,bytes4,bytes)",
						name: "OffchainLookup",
						args: o.errorArgs
					}
				});
				let s = await this.ccipReadFetch(r, o.calldata, o.urls);
				u(s != null, "CCIP Read failed to fetch data", "OFFCHAIN_FAULT", {
					reason: "FETCH_FAILED",
					transaction: r,
					info: {
						data: e.data,
						errorArgs: o.errorArgs
					}
				});
				let c = {
					to: a,
					data: C([o.selector, Sd([s, o.extraData])])
				};
				this.emit("debug", {
					action: "sendCcipReadCall",
					transaction: c
				});
				try {
					let e = await this.#g(c, t, n + 1);
					return this.emit("debug", {
						action: "receiveCcipReadCallResult",
						transaction: Object.assign({}, c),
						result: e
					}), e;
				} catch (e) {
					throw this.emit("debug", {
						action: "receiveCcipReadCallError",
						transaction: Object.assign({}, c),
						error: e
					}), e;
				}
			}
			throw e;
		}
	}
	async #_(e) {
		let { value: t } = await i({
			network: this.getNetwork(),
			value: e
		});
		return t;
	}
	async call(e) {
		let { tx: t, blockTag: n } = await i({
			tx: this._getTransactionRequest(e),
			blockTag: this._getBlockTag(e.blockTag)
		});
		return await this.#_(this.#g(t, n, e.enableCcipRead ? 0 : -1));
	}
	async #v(e, t, n) {
		let r = this._getAddress(t), i = this._getBlockTag(n);
		return (typeof r != "string" || typeof i != "string") && ([r, i] = await Promise.all([r, i])), await this.#_(this.#h(Object.assign(e, {
			address: r,
			blockTag: i
		})));
	}
	async getBalance(e, t) {
		return F(await this.#v({ method: "getBalance" }, e, t), "%response");
	}
	async getTransactionCount(e, t) {
		return z(await this.#v({ method: "getTransactionCount" }, e, t), "%response");
	}
	async getCode(e, t) {
		return S(await this.#v({ method: "getCode" }, e, t));
	}
	async getStorage(e, t, n) {
		let r = F(t, "position");
		return S(await this.#v({
			method: "getStorage",
			position: r
		}, e, n));
	}
	async broadcastTransaction(e) {
		let { blockNumber: t, hash: n, network: r } = await i({
			blockNumber: this.getBlockNumber(),
			hash: this._perform({
				method: "broadcastTransaction",
				signedTransaction: e
			}),
			network: this.getNetwork()
		}), a = ho.from(e);
		if (a.hash !== n) throw Error("@TODO: the returned hash did not match");
		return this._wrapTransactionResponse(a, r).replaceableTransaction(t);
	}
	async #y(e, t) {
		if (y(e, 32)) return await this.#h({
			method: "getBlock",
			blockHash: e,
			includeTransactions: t
		});
		let n = this._getBlockTag(e);
		return typeof n != "string" && (n = await n), await this.#h({
			method: "getBlock",
			blockTag: n,
			includeTransactions: t
		});
	}
	async getBlock(e, t) {
		let { network: n, params: r } = await i({
			network: this.getNetwork(),
			params: this.#y(e, !!t)
		});
		return r == null ? null : this._wrapBlock(r, n);
	}
	async getTransaction(e) {
		let { network: t, params: n } = await i({
			network: this.getNetwork(),
			params: this.#h({
				method: "getTransaction",
				hash: e
			})
		});
		return n == null ? null : this._wrapTransactionResponse(n, t);
	}
	async getTransactionReceipt(e) {
		let { network: t, params: n } = await i({
			network: this.getNetwork(),
			params: this.#h({
				method: "getTransactionReceipt",
				hash: e
			})
		});
		if (n == null) return null;
		if (n.gasPrice == null && n.effectiveGasPrice == null) {
			let t = await this.#h({
				method: "getTransaction",
				hash: e
			});
			if (t == null) throw Error("report this; could not find tx or effectiveGasPrice");
			n.effectiveGasPrice = t.gasPrice;
		}
		return this._wrapTransactionReceipt(n, t);
	}
	async getTransactionResult(e) {
		let { result: t } = await i({
			network: this.getNetwork(),
			result: this.#h({
				method: "getTransactionResult",
				hash: e
			})
		});
		return t == null ? null : S(t);
	}
	async getLogs(e) {
		let t = this._getFilter(e);
		cd(t) && (t = await t);
		let { network: n, params: r } = await i({
			network: this.getNetwork(),
			params: this.#h({
				method: "getLogs",
				filter: t
			})
		});
		return r.map((e) => this._wrapLog(e, n));
	}
	_getProvider(e) {
		u(!1, "provider cannot connect to target network", "UNSUPPORTED_OPERATION", { operation: "_getProvider()" });
	}
	async getResolver(e) {
		return await Su.fromName(this, e);
	}
	async getAvatar(e) {
		let t = await this.getResolver(e);
		return t ? await t.getAvatar() : null;
	}
	async resolveName(e, t) {
		let n = await this.getResolver(e);
		return n ? await n.getAddress(t) : null;
	}
	async lookupAddress(e, t) {
		return await Su.lookupAddress(this, e, t);
	}
	async waitForTransaction(e, t, n) {
		let r = t ?? 1;
		return r === 0 ? this.getTransactionReceipt(e) : new Promise(async (t, i) => {
			let a = null, o = (async (n) => {
				try {
					let i = await this.getTransactionReceipt(e);
					if (i != null && n - i.blockNumber + 1 >= r) {
						t(i), a &&= (clearTimeout(a), null);
						return;
					}
				} catch (e) {
					console.log("EEE", e);
				}
				this.once("block", o);
			});
			n != null && (a = setTimeout(() => {
				a != null && (a = null, this.off("block", o), i(l("timeout", "TIMEOUT", { reason: "timeout" })));
			}, n)), o(await this.getBlockNumber());
		});
	}
	async waitForBlock(e) {
		u(!1, "not implemented yet", "NOT_IMPLEMENTED", { operation: "waitForBlock" });
	}
	_clearTimeout(e) {
		let t = this.#l.get(e);
		t && (t.timer && clearTimeout(t.timer), this.#l.delete(e));
	}
	_setTimeout(e, t) {
		t ??= 0;
		let n = this.#c++, r = () => {
			this.#l.delete(n), e();
		};
		if (this.paused) this.#l.set(n, {
			timer: null,
			func: r,
			time: t
		});
		else {
			let e = setTimeout(r, t);
			this.#l.set(n, {
				timer: e,
				func: r,
				time: md()
			});
		}
		return n;
	}
	_forEachSubscriber(e) {
		for (let t of this.#e.values()) e(t.subscriber);
	}
	_getSubscriber(e) {
		switch (e.type) {
			case "debug":
			case "error":
			case "network": return new ud(e.type);
			case "block": {
				let e = new $u(this);
				return e.pollingInterval = this.pollingInterval, e;
			}
			case "safe":
			case "finalized": return new td(this, e.type);
			case "event": return new id(this, e.filter);
			case "transaction": return new rd(this, e.hash);
			case "orphan": return new nd(this, e.filter);
		}
		throw Error(`unsupported event: ${e.type}`);
	}
	_recoverSubscriber(e, t) {
		for (let n of this.#e.values()) if (n.subscriber === e) {
			n.started && n.subscriber.stop(), n.subscriber = t, n.started && t.start(), this.#n != null && t.pause(this.#n);
			break;
		}
	}
	async #b(e, t) {
		let n = await pd(e, this);
		return n.type === "event" && t && t.length > 0 && t[0].removed === !0 && (n = await pd({
			orphan: "drop-log",
			log: t[0]
		}, this)), this.#e.get(n.tag) || null;
	}
	async #x(e) {
		let t = await pd(e, this), n = t.tag, r = this.#e.get(n);
		return r || (r = {
			subscriber: this._getSubscriber(t),
			tag: n,
			addressableMap: /* @__PURE__ */ new WeakMap(),
			nameMap: /* @__PURE__ */ new Map(),
			started: !1,
			listeners: []
		}, this.#e.set(n, r)), r;
	}
	async on(e, t) {
		let n = await this.#x(e);
		return n.listeners.push({
			listener: t,
			once: !1
		}), n.started || (n.subscriber.start(), n.started = !0, this.#n != null && n.subscriber.pause(this.#n)), this;
	}
	async once(e, t) {
		let n = await this.#x(e);
		return n.listeners.push({
			listener: t,
			once: !0
		}), n.started || (n.subscriber.start(), n.started = !0, this.#n != null && n.subscriber.pause(this.#n)), this;
	}
	async emit(e, ...t) {
		let n = await this.#b(e, t);
		if (!n || n.listeners.length === 0) return !1;
		let r = n.listeners.length;
		return n.listeners = n.listeners.filter(({ listener: n, once: r }) => {
			let i = new se(this, r ? null : n, e);
			try {
				n.call(this, ...t, i);
			} catch {}
			return !r;
		}), n.listeners.length === 0 && (n.started && n.subscriber.stop(), this.#e.delete(n.tag)), r > 0;
	}
	async listenerCount(e) {
		if (e) {
			let t = await this.#b(e);
			return t ? t.listeners.length : 0;
		}
		let t = 0;
		for (let { listeners: e } of this.#e.values()) t += e.length;
		return t;
	}
	async listeners(e) {
		if (e) {
			let t = await this.#b(e);
			return t ? t.listeners.map(({ listener: e }) => e) : [];
		}
		let t = [];
		for (let { listeners: e } of this.#e.values()) t = t.concat(e.map(({ listener: e }) => e));
		return t;
	}
	async off(e, t) {
		let n = await this.#b(e);
		if (!n) return this;
		if (t) {
			let e = n.listeners.map(({ listener: e }) => e).indexOf(t);
			e >= 0 && n.listeners.splice(e, 1);
		}
		return (!t || n.listeners.length === 0) && (n.started && n.subscriber.stop(), this.#e.delete(n.tag)), this;
	}
	async removeAllListeners(e) {
		if (e) {
			let { tag: t, started: n, subscriber: r } = await this.#x(e);
			n && r.stop(), this.#e.delete(t);
		} else for (let [e, { started: t, subscriber: n }] of this.#e) t && n.stop(), this.#e.delete(e);
		return this;
	}
	async addListener(e, t) {
		return await this.on(e, t);
	}
	async removeListener(e, t) {
		return this.off(e, t);
	}
	get destroyed() {
		return this.#r;
	}
	destroy() {
		this.removeAllListeners();
		for (let e of this.#l.keys()) this._clearTimeout(e);
		this.#r = !0;
	}
	get paused() {
		return this.#n != null;
	}
	set paused(e) {
		!!e !== this.paused && (this.paused ? this.resume() : this.pause(!1));
	}
	pause(e) {
		if (this.#s = -1, this.#n != null) {
			if (this.#n == !!e) return;
			u(!1, "cannot change pause type; resume first", "UNSUPPORTED_OPERATION", { operation: "pause" });
		}
		this._forEachSubscriber((t) => t.pause(e)), this.#n = !!e;
		for (let e of this.#l.values()) e.timer && clearTimeout(e.timer), e.time = md() - e.time;
	}
	resume() {
		if (this.#n != null) {
			this._forEachSubscriber((e) => e.resume()), this.#n = null;
			for (let e of this.#l.values()) {
				let t = e.time;
				t < 0 && (t = 0), e.time = md(), setTimeout(e.func, t);
			}
		}
	}
};
function _d(e, t) {
	try {
		let n = vd(e, t);
		if (n) return me(n);
	} catch {}
	return null;
}
function vd(e, t) {
	if (e === "0x") return null;
	try {
		let n = z(T(e, t, t + 32)), r = z(T(e, n, n + 32));
		return T(e, n + 32, n + 32 + r);
	} catch {}
	return null;
}
function yd(e) {
	let t = B(e);
	if (t.length > 32) throw Error("internal; should not happen");
	let n = /* @__PURE__ */ new Uint8Array(32);
	return n.set(t, 32 - t.length), n;
}
function bd(e) {
	if (e.length % 32 == 0) return e;
	let t = new Uint8Array(Math.ceil(e.length / 32) * 32);
	return t.set(e), t;
}
var xd = new Uint8Array([]);
function Sd(e) {
	let t = [], n = 0;
	for (let r = 0; r < e.length; r++) t.push(xd), n += 32;
	for (let r = 0; r < e.length; r++) {
		let i = _(e[r]);
		t[r] = yd(n), t.push(yd(i.length)), t.push(bd(i)), n += 32 + Math.ceil(i.length / 32) * 32;
	}
	return C(t);
}
var Cd = "0x0000000000000000000000000000000000000000000000000000000000000000";
function wd(e) {
	let t = {
		sender: "",
		urls: [],
		calldata: "",
		selector: "",
		extraData: "",
		errorArgs: []
	};
	u(w(e) >= 160, "insufficient OffchainLookup data", "OFFCHAIN_FAULT", { reason: "insufficient OffchainLookup data" });
	let n = T(e, 0, 32);
	u(T(n, 0, 12) === T(Cd, 0, 12), "corrupt OffchainLookup sender", "OFFCHAIN_FAULT", { reason: "corrupt OffchainLookup sender" }), t.sender = T(n, 12);
	try {
		let n = [], r = z(T(e, 32, 64)), i = z(T(e, r, r + 32)), a = T(e, r + 32);
		for (let e = 0; e < i; e++) {
			let t = _d(a, e * 32);
			if (t == null) throw Error("abort");
			n.push(t);
		}
		t.urls = n;
	} catch {
		u(!1, "corrupt OffchainLookup urls", "OFFCHAIN_FAULT", { reason: "corrupt OffchainLookup urls" });
	}
	try {
		let n = vd(e, 64);
		if (n == null) throw Error("abort");
		t.calldata = n;
	} catch {
		u(!1, "corrupt OffchainLookup calldata", "OFFCHAIN_FAULT", { reason: "corrupt OffchainLookup calldata" });
	}
	u(T(e, 100, 128) === T(Cd, 0, 28), "corrupt OffchainLookup callbaackSelector", "OFFCHAIN_FAULT", { reason: "corrupt OffchainLookup callbaackSelector" }), t.selector = T(e, 96, 100);
	try {
		let n = vd(e, 128);
		if (n == null) throw Error("abort");
		t.extraData = n;
	} catch {
		u(!1, "corrupt OffchainLookup extraData", "OFFCHAIN_FAULT", { reason: "corrupt OffchainLookup extraData" });
	}
	return t.errorArgs = "sender,urls,calldata,selector,extraData".split(/,/).map((e) => t[e]), t;
}
//#endregion
//#region node_modules/ethers/lib.esm/providers/abstract-signer.js
function Td(e, t) {
	if (e.provider) return e.provider;
	u(!1, "missing provider", "UNSUPPORTED_OPERATION", { operation: t });
}
async function Ed(e, t) {
	let n = El(t);
	if (n.to != null && (n.to = pa(n.to, e)), n.from != null) {
		let t = n.from;
		n.from = Promise.all([e.getAddress(), pa(t, e)]).then(([e, t]) => (d(e.toLowerCase() === t.toLowerCase(), "transaction from mismatch", "tx.from", t), e));
	} else n.from = e.getAddress();
	return await i(n);
}
var Dd = class {
	provider;
	constructor(e) {
		a(this, { provider: e || null });
	}
	async getNonce(e) {
		return Td(this, "getTransactionCount").getTransactionCount(await this.getAddress(), e);
	}
	async populateCall(e) {
		return await Ed(this, e);
	}
	async populateTransaction(e) {
		let t = Td(this, "populateTransaction"), n = await Ed(this, e);
		n.nonce ??= await this.getNonce("pending"), n.gasLimit ??= await this.estimateGas(n);
		let r = await this.provider.getNetwork();
		n.chainId == null ? n.chainId = r.chainId : d(F(n.chainId) === r.chainId, "transaction chainId mismatch", "tx.chainId", e.chainId);
		let a = n.maxFeePerGas != null || n.maxPriorityFeePerGas != null;
		if (n.gasPrice != null && (n.type === 2 || a) ? d(!1, "eip-1559 transaction do not support gasPrice", "tx", e) : (n.type === 0 || n.type === 1) && a && d(!1, "pre-eip-1559 transaction do not support maxFeePerGas/maxPriorityFeePerGas", "tx", e), (n.type === 2 || n.type == null) && n.maxFeePerGas != null && n.maxPriorityFeePerGas != null) n.type = 2;
		else if (n.type === 0 || n.type === 1) {
			let e = await t.getFeeData();
			u(e.gasPrice != null, "network does not support gasPrice", "UNSUPPORTED_OPERATION", { operation: "getGasPrice" }), n.gasPrice ??= e.gasPrice;
		} else {
			let e = await t.getFeeData();
			if (n.type == null) {
				if (e.maxFeePerGas != null && e.maxPriorityFeePerGas != null) {
					if (n.type = n.authorizationList && n.authorizationList.length ? 4 : 2, n.gasPrice != null) {
						let e = n.gasPrice;
						delete n.gasPrice, n.maxFeePerGas = e, n.maxPriorityFeePerGas = e;
					} else n.maxFeePerGas ??= e.maxFeePerGas, n.maxPriorityFeePerGas ??= e.maxPriorityFeePerGas;
				} else e.gasPrice == null ? u(!1, "failed to get consistent fee data", "UNSUPPORTED_OPERATION", { operation: "signer.getFeeData" }) : (u(!a, "network does not support EIP-1559", "UNSUPPORTED_OPERATION", { operation: "populateTransaction" }), n.gasPrice ??= e.gasPrice, n.type = 0);
			} else (n.type === 2 || n.type === 3 || n.type === 4) && (n.maxFeePerGas ??= e.maxFeePerGas, n.maxPriorityFeePerGas ??= e.maxPriorityFeePerGas);
		}
		return await i(n);
	}
	async populateAuthorization(e) {
		let t = Object.assign({}, e);
		return t.chainId ??= (await Td(this, "getNetwork").getNetwork()).chainId, t.nonce ??= await this.getNonce(), t;
	}
	async estimateGas(e) {
		return Td(this, "estimateGas").estimateGas(await this.populateCall(e));
	}
	async call(e) {
		return Td(this, "call").call(await this.populateCall(e));
	}
	async resolveName(e) {
		return await Td(this, "resolveName").resolveName(e);
	}
	async sendTransaction(e) {
		let t = Td(this, "sendTransaction"), n = await this.populateTransaction(e);
		delete n.from;
		let r = ho.from(n);
		return await t.broadcastTransaction(await this.signTransaction(r));
	}
	authorize(e) {
		u(!1, "authorization not implemented for this signer", "UNSUPPORTED_OPERATION", { operation: "authorize" });
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/providers/subscriber-filterid.js
function Od(e) {
	return JSON.parse(JSON.stringify(e));
}
var kd = class {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	constructor(e) {
		this.#e = e, this.#t = null, this.#n = this.#o.bind(this), this.#r = !1, this.#i = null, this.#a = !1;
	}
	_subscribe(e) {
		throw Error("subclasses must override this");
	}
	_emitResults(e, t) {
		throw Error("subclasses must override this");
	}
	_recover(e) {
		throw Error("subclasses must override this");
	}
	async #o(e) {
		try {
			this.#t ??= this._subscribe(this.#e);
			let e = null;
			try {
				e = await this.#t;
			} catch (e) {
				if (!s(e, "UNSUPPORTED_OPERATION") || e.operation !== "eth_newFilter") throw e;
			}
			if (e == null) {
				this.#t = null, this.#e._recoverSubscriber(this, this._recover(this.#e));
				return;
			}
			let t = await this.#e.getNetwork();
			if (this.#i ||= t, this.#i.chainId !== t.chainId) throw Error("chaid changed");
			if (this.#a) return;
			let n = await this.#e.send("eth_getFilterChanges", [e]);
			await this._emitResults(this.#e, n);
		} catch (e) {
			console.log("@TODO", e);
		}
		this.#e.once("block", this.#n);
	}
	#s() {
		let e = this.#t;
		e && (this.#t = null, e.then((e) => {
			this.#e.destroyed || this.#e.send("eth_uninstallFilter", [e]);
		}));
	}
	start() {
		this.#r || (this.#r = !0, this.#o(-2));
	}
	stop() {
		this.#r && (this.#r = !1, this.#a = !0, this.#s(), this.#e.off("block", this.#n));
	}
	pause(e) {
		e && this.#s(), this.#e.off("block", this.#n);
	}
	resume() {
		this.start();
	}
}, Ad = class extends kd {
	#e;
	constructor(e, t) {
		super(e), this.#e = Od(t);
	}
	_recover(e) {
		return new id(e, this.#e);
	}
	async _subscribe(e) {
		return await e.send("eth_newFilter", [this.#e]);
	}
	async _emitResults(e, t) {
		for (let n of t) e.emit(this.#e, e._wrapLog(n, e._network));
	}
}, jd = class extends kd {
	async _subscribe(e) {
		return await e.send("eth_newPendingTransactionFilter", []);
	}
	async _emitResults(e, t) {
		for (let n of t) e.emit("pending", n);
	}
}, Md = "bigint,boolean,function,number,string,symbol".split(/,/g);
function Nd(e) {
	if (e == null || Md.indexOf(typeof e) >= 0 || typeof e.getAddress == "function") return e;
	if (Array.isArray(e)) return e.map(Nd);
	if (typeof e == "object") return Object.keys(e).reduce((t, n) => (t[n] = e[n], t), {});
	throw Error(`should not happen: ${e} (${typeof e})`);
}
function Pd(e) {
	return new Promise((t) => {
		setTimeout(t, e);
	});
}
function Fd(e) {
	return e && e.toLowerCase();
}
function Id(e) {
	return e && typeof e.pollingInterval == "number";
}
var Ld = {
	polling: !1,
	staticNetwork: null,
	batchStallTime: 10,
	batchMaxSize: 1 << 20,
	batchMaxCount: 100,
	cacheTimeout: 250,
	pollingInterval: 4e3
}, Rd = class extends Dd {
	address;
	constructor(e, t) {
		super(e), t = K(t), a(this, { address: t });
	}
	connect(e) {
		u(!1, "cannot reconnect JsonRpcSigner", "UNSUPPORTED_OPERATION", { operation: "signer.connect" });
	}
	async getAddress() {
		return this.address;
	}
	async populateTransaction(e) {
		return await this.populateCall(e);
	}
	async sendUncheckedTransaction(e) {
		let t = Nd(e), n = [];
		if (t.from) {
			let r = t.from;
			n.push((async () => {
				let n = await pa(r, this.provider);
				d(n != null && n.toLowerCase() === this.address.toLowerCase(), "from address mismatch", "transaction", e), t.from = n;
			})());
		} else t.from = this.address;
		if (t.gasLimit ?? n.push((async () => {
			t.gasLimit = await this.provider.estimateGas({
				...t,
				from: this.address
			});
		})()), t.to != null) {
			let e = t.to;
			n.push((async () => {
				t.to = await pa(e, this.provider);
			})());
		}
		n.length && await Promise.all(n);
		let r = this.provider.getRpcTransaction(t);
		return this.provider.send("eth_sendTransaction", [r]);
	}
	async sendTransaction(e) {
		let t = await this.provider.getBlockNumber(), n = await this.sendUncheckedTransaction(e);
		return await new Promise((e, r) => {
			let i = [1e3, 100], a = 0, o = async () => {
				try {
					let r = await this.provider.getTransaction(n);
					if (r != null) {
						e(r.replaceableTransaction(t));
						return;
					}
				} catch (e) {
					if (s(e, "CANCELLED") || s(e, "BAD_DATA") || s(e, "NETWORK_ERROR") || s(e, "UNSUPPORTED_OPERATION")) {
						e.info ??= {}, e.info.sendTransactionHash = n, r(e);
						return;
					}
					if (s(e, "INVALID_ARGUMENT") && (a++, e.info ??= {}, e.info.sendTransactionHash = n, a > 10)) {
						r(e);
						return;
					}
					this.provider.emit("error", l("failed to fetch transation after sending (will try again)", "UNKNOWN_ERROR", { error: e }));
				}
				this.provider._setTimeout(() => {
					o();
				}, i.pop() || 4e3);
			};
			o();
		});
	}
	async signTransaction(e) {
		let t = Nd(e);
		if (t.from) {
			let n = await pa(t.from, this.provider);
			d(n != null && n.toLowerCase() === this.address.toLowerCase(), "from address mismatch", "transaction", e), t.from = n;
		} else t.from = this.address;
		let n = this.provider.getRpcTransaction(t);
		return await this.provider.send("eth_signTransaction", [n]);
	}
	async signMessage(e) {
		let t = typeof e == "string" ? H(e) : e;
		return await this.provider.send("personal_sign", [S(t), this.address.toLowerCase()]);
	}
	async signTypedData(e, t, n) {
		let r = Nd(n), i = await gc.resolveNames(e, t, r, async (e) => {
			let t = await pa(e);
			return d(t != null, "TypedData does not support null address", "value", e), t;
		});
		return await this.provider.send("eth_signTypedData_v4", [this.address.toLowerCase(), JSON.stringify(gc.getPayload(i.domain, t, i.value))]);
	}
	async unlock(e) {
		return this.provider.send("personal_unlockAccount", [
			this.address.toLowerCase(),
			e,
			null
		]);
	}
	async _legacySignMessage(e) {
		let t = typeof e == "string" ? H(e) : e;
		return await this.provider.send("eth_sign", [this.address.toLowerCase(), S(t)]);
	}
}, zd = class extends gd {
	#e;
	#t;
	#n;
	#r;
	#i;
	#a;
	#o;
	#s() {
		if (this.#r) return;
		let e = this._getOption("batchMaxCount") === 1 ? 0 : this._getOption("batchStallTime");
		this.#r = setTimeout(() => {
			this.#r = null;
			let e = this.#n;
			for (this.#n = []; e.length;) {
				let t = [e.shift()];
				for (; e.length && t.length !== this.#e.batchMaxCount;) if (t.push(e.shift()), JSON.stringify(t.map((e) => e.payload)).length > this.#e.batchMaxSize) {
					e.unshift(t.pop());
					break;
				}
				(async () => {
					let e = t.length === 1 ? t[0].payload : t.map((e) => e.payload);
					this.emit("debug", {
						action: "sendRpcPayload",
						payload: e
					});
					try {
						let n = await this._send(e);
						this.emit("debug", {
							action: "receiveRpcResult",
							result: n
						});
						for (let { resolve: e, reject: r, payload: i } of t) {
							if (this.destroyed) {
								r(l("provider destroyed; cancelled request", "UNSUPPORTED_OPERATION", { operation: i.method }));
								continue;
							}
							let t = n.filter((e) => e.id === i.id)[0];
							if (t == null) {
								let e = l("missing response for request", "BAD_DATA", {
									value: n,
									info: { payload: i }
								});
								this.emit("error", e), r(e);
								continue;
							}
							if ("error" in t) {
								r(this.getRpcError(i, t));
								continue;
							}
							e(t.result);
						}
					} catch (e) {
						this.emit("debug", {
							action: "receiveRpcError",
							error: e
						});
						for (let { reject: n } of t) n(e);
					}
				})();
			}
		}, e);
	}
	constructor(e, t) {
		super(e, t), this.#t = 1, this.#e = Object.assign({}, Ld, t || {}), this.#n = [], this.#r = null, this.#a = null, this.#o = null;
		{
			let e = null, t = new Promise((t) => {
				e = t;
			});
			this.#i = {
				promise: t,
				resolve: e
			};
		}
		let n = this._getOption("staticNetwork");
		typeof n == "boolean" ? (d(!n || e !== "any", "staticNetwork cannot be used on special network 'any'", "options", t), n && e != null && (this.#a = qu.from(e))) : n && (d(e == null || n.matches(e), "staticNetwork MUST match network object", "options", t), this.#a = n);
	}
	_getOption(e) {
		return this.#e[e];
	}
	get _network() {
		return u(this.#a, "network is not available yet", "NETWORK_ERROR"), this.#a;
	}
	async _perform(e) {
		if (e.method === "call" || e.method === "estimateGas") {
			let t = e.transaction;
			if (t && t.type != null && F(t.type) && t.maxFeePerGas == null && t.maxPriorityFeePerGas == null) {
				let n = await this.getFeeData();
				n.maxFeePerGas == null && n.maxPriorityFeePerGas == null && (e = Object.assign({}, e, { transaction: Object.assign({}, t, { type: void 0 }) }));
			}
		}
		let t = this.getRpcRequest(e);
		return t == null ? super._perform(e) : await this.send(t.method, t.args);
	}
	async _detectNetwork() {
		let e = this._getOption("staticNetwork");
		if (e) {
			if (e === !0) {
				if (this.#a) return this.#a;
			} else return e;
		}
		return this.#o ? await this.#o : this.ready ? (this.#o = (async () => {
			try {
				let e = qu.from(F(await this.send("eth_chainId", [])));
				return this.#o = null, e;
			} catch (e) {
				throw this.#o = null, e;
			}
		})(), await this.#o) : (this.#o = (async () => {
			let e = {
				id: this.#t++,
				method: "eth_chainId",
				params: [],
				jsonrpc: "2.0"
			};
			this.emit("debug", {
				action: "sendRpcPayload",
				payload: e
			});
			let t;
			try {
				t = (await this._send(e))[0], this.#o = null;
			} catch (e) {
				throw this.#o = null, this.emit("debug", {
					action: "receiveRpcError",
					error: e
				}), e;
			}
			if (this.emit("debug", {
				action: "receiveRpcResult",
				result: t
			}), "result" in t) return qu.from(F(t.result));
			throw this.getRpcError(e, t);
		})(), await this.#o);
	}
	_start() {
		this.#i != null && this.#i.resolve != null && (this.#i.resolve(), this.#i = null, (async () => {
			for (; this.#a == null && !this.destroyed;) try {
				this.#a = await this._detectNetwork();
			} catch (e) {
				if (this.destroyed) break;
				console.log("JsonRpcProvider failed to detect network and cannot start up; retry in 1s (perhaps the URL is wrong or the node is not started)"), this.emit("error", l("failed to bootstrap network detection", "NETWORK_ERROR", {
					event: "initial-network-discovery",
					info: { error: e }
				})), await Pd(1e3);
			}
			this.#s();
		})());
	}
	async _waitUntilReady() {
		if (this.#i != null) return await this.#i.promise;
	}
	_getSubscriber(e) {
		return e.type === "pending" ? new jd(this) : e.type === "event" ? this._getOption("polling") ? new id(this, e.filter) : new Ad(this, e.filter) : e.type === "orphan" && e.filter.orphan === "drop-log" ? new ud("orphan") : super._getSubscriber(e);
	}
	get ready() {
		return this.#i == null;
	}
	getRpcTransaction(e) {
		let t = {};
		return [
			"chainId",
			"gasLimit",
			"gasPrice",
			"type",
			"maxFeePerGas",
			"maxPriorityFeePerGas",
			"nonce",
			"value"
		].forEach((n) => {
			if (e[n] == null) return;
			let r = n;
			n === "gasLimit" && (r = "gas"), t[r] = V(F(e[n], `tx.${n}`));
		}), [
			"from",
			"to",
			"data"
		].forEach((n) => {
			e[n] != null && (t[n] = S(e[n]));
		}), e.accessList && (t.accessList = Fa(e.accessList)), e.blobVersionedHashes && (t.blobVersionedHashes = e.blobVersionedHashes.map((e) => e.toLowerCase())), e.authorizationList && (t.authorizationList = e.authorizationList.map((e) => {
			let t = Ia(e);
			return {
				address: t.address,
				nonce: V(t.nonce),
				chainId: V(t.chainId),
				yParity: V(t.signature.yParity),
				r: V(t.signature.r),
				s: V(t.signature.s)
			};
		})), t;
	}
	getRpcRequest(e) {
		switch (e.method) {
			case "chainId": return {
				method: "eth_chainId",
				args: []
			};
			case "getBlockNumber": return {
				method: "eth_blockNumber",
				args: []
			};
			case "getGasPrice": return {
				method: "eth_gasPrice",
				args: []
			};
			case "getPriorityFee": return {
				method: "eth_maxPriorityFeePerGas",
				args: []
			};
			case "getBalance": return {
				method: "eth_getBalance",
				args: [Fd(e.address), e.blockTag]
			};
			case "getTransactionCount": return {
				method: "eth_getTransactionCount",
				args: [Fd(e.address), e.blockTag]
			};
			case "getCode": return {
				method: "eth_getCode",
				args: [Fd(e.address), e.blockTag]
			};
			case "getStorage": return {
				method: "eth_getStorageAt",
				args: [
					Fd(e.address),
					"0x" + e.position.toString(16),
					e.blockTag
				]
			};
			case "broadcastTransaction": return {
				method: "eth_sendRawTransaction",
				args: [e.signedTransaction]
			};
			case "getBlock":
				if ("blockTag" in e) return {
					method: "eth_getBlockByNumber",
					args: [e.blockTag, !!e.includeTransactions]
				};
				if ("blockHash" in e) return {
					method: "eth_getBlockByHash",
					args: [e.blockHash, !!e.includeTransactions]
				};
				break;
			case "getTransaction": return {
				method: "eth_getTransactionByHash",
				args: [e.hash]
			};
			case "getTransactionReceipt": return {
				method: "eth_getTransactionReceipt",
				args: [e.hash]
			};
			case "call": return {
				method: "eth_call",
				args: [this.getRpcTransaction(e.transaction), e.blockTag]
			};
			case "estimateGas": return {
				method: "eth_estimateGas",
				args: [this.getRpcTransaction(e.transaction)]
			};
			case "getLogs": return e.filter && e.filter.address != null && (Array.isArray(e.filter.address) ? e.filter.address = e.filter.address.map(Fd) : e.filter.address = Fd(e.filter.address)), {
				method: "eth_getLogs",
				args: [e.filter]
			};
		}
		return null;
	}
	getRpcError(e, t) {
		let { method: n } = e, { error: r } = t;
		if (n === "eth_estimateGas" && r.message) {
			let t = r.message;
			if (!t.match(/revert/i) && t.match(/insufficient funds/i)) return l("insufficient funds", "INSUFFICIENT_FUNDS", {
				transaction: e.params[0],
				info: {
					payload: e,
					error: r
				}
			});
			if (t.match(/nonce/i) && t.match(/too low/i)) return l("nonce has already been used", "NONCE_EXPIRED", {
				transaction: e.params[0],
				info: {
					payload: e,
					error: r
				}
			});
		}
		if (n === "eth_call" || n === "eth_estimateGas") {
			let t = Hd(r), i = ml.getBuiltinCallException(n === "eth_call" ? "call" : "estimateGas", e.params[0], t ? t.data : null);
			return i.info = {
				error: r,
				payload: e
			}, i;
		}
		let i = JSON.stringify(Wd(r));
		if (typeof r.message == "string" && r.message.match(/user denied|ethers-user-denied/i)) return l("user rejected action", "ACTION_REJECTED", {
			action: {
				eth_sign: "signMessage",
				personal_sign: "signMessage",
				eth_signTypedData_v4: "signTypedData",
				eth_signTransaction: "signTransaction",
				eth_sendTransaction: "sendTransaction",
				eth_requestAccounts: "requestAccess",
				wallet_requestAccounts: "requestAccess"
			}[n] || "unknown",
			reason: "rejected",
			info: {
				payload: e,
				error: r
			}
		});
		if (n === "eth_sendRawTransaction" || n === "eth_sendTransaction") {
			let t = e.params[0];
			if (i.match(/insufficient funds|base fee exceeds gas limit/i)) return l("insufficient funds for intrinsic transaction cost", "INSUFFICIENT_FUNDS", {
				transaction: t,
				info: { error: r }
			});
			if (i.match(/nonce/i) && i.match(/too low/i)) return l("nonce has already been used", "NONCE_EXPIRED", {
				transaction: t,
				info: { error: r }
			});
			if (i.match(/replacement transaction/i) && i.match(/underpriced/i)) return l("replacement fee too low", "REPLACEMENT_UNDERPRICED", {
				transaction: t,
				info: { error: r }
			});
			if (i.match(/only replay-protected/i)) return l("legacy pre-eip-155 transactions not supported", "UNSUPPORTED_OPERATION", {
				operation: n,
				info: {
					transaction: t,
					info: { error: r }
				}
			});
		}
		let a = !!i.match(/the method .* does not exist/i);
		return a || r && r.details && r.details.startsWith("Unauthorized method:") && (a = !0), a ? l("unsupported operation", "UNSUPPORTED_OPERATION", {
			operation: e.method,
			info: {
				error: r,
				payload: e
			}
		}) : l("could not coalesce error", "UNKNOWN_ERROR", {
			error: r,
			payload: e
		});
	}
	send(e, t) {
		if (this.destroyed) return Promise.reject(l("provider destroyed; cancelled request", "UNSUPPORTED_OPERATION", { operation: e }));
		let n = this.#t++, r = new Promise((r, i) => {
			this.#n.push({
				resolve: r,
				reject: i,
				payload: {
					method: e,
					params: t,
					id: n,
					jsonrpc: "2.0"
				}
			});
		});
		return this.#s(), r;
	}
	async getSigner(e) {
		e ??= 0;
		let t = this.send("eth_accounts", []);
		if (typeof e == "number") {
			let n = await t;
			if (e >= n.length) throw Error("no such account");
			return new Rd(this, n[e]);
		}
		let { accounts: n } = await i({
			network: this.getNetwork(),
			accounts: t
		});
		e = K(e);
		for (let t of n) if (K(t) === e) return new Rd(this, e);
		throw Error("invalid account");
	}
	async listAccounts() {
		return (await this.send("eth_accounts", [])).map((e) => new Rd(this, e));
	}
	destroy() {
		this.#r &&= (clearTimeout(this.#r), null);
		for (let { payload: e, reject: t } of this.#n) t(l("provider destroyed; cancelled request", "UNSUPPORTED_OPERATION", { operation: e.method }));
		this.#n = [], super.destroy();
	}
}, Bd = class extends zd {
	#e;
	constructor(e, t) {
		super(e, t);
		let n = this._getOption("pollingInterval");
		n ??= Ld.pollingInterval, this.#e = n;
	}
	_getSubscriber(e) {
		let t = super._getSubscriber(e);
		return Id(t) && (t.pollingInterval = this.#e), t;
	}
	get pollingInterval() {
		return this.#e;
	}
	set pollingInterval(e) {
		if (!Number.isInteger(e) || e < 0) throw Error("invalid interval");
		this.#e = e, this._forEachSubscriber((e) => {
			Id(e) && (e.pollingInterval = this.#e);
		});
	}
}, Vd = class extends Bd {
	#e;
	constructor(e, t, n) {
		e ??= "http://localhost:8545", super(t, n), this.#e = typeof e == "string" ? new Oe(e) : e.clone();
	}
	_getConnection() {
		return this.#e.clone();
	}
	async send(e, t) {
		return await this._start(), await super.send(e, t);
	}
	async _send(e) {
		let t = this._getConnection();
		t.body = JSON.stringify(e), t.setHeader("content-type", "application/json");
		let n = await t.send();
		n.assertOk();
		let r = n.bodyJson;
		return Array.isArray(r) || (r = [r]), r;
	}
};
function Hd(e) {
	if (e == null) return null;
	if (typeof e.message == "string" && e.message.match(/revert/i) && y(e.data)) return {
		message: e.message,
		data: e.data
	};
	if (typeof e == "object") {
		for (let t in e) {
			let n = Hd(e[t]);
			if (n) return n;
		}
		return null;
	}
	if (typeof e == "string") try {
		return Hd(JSON.parse(e));
	} catch {}
	return null;
}
function Ud(e, t) {
	if (e != null) {
		if (typeof e.message == "string" && t.push(e.message), typeof e == "object") for (let n in e) Ud(e[n], t);
		if (typeof e == "string") try {
			return Ud(JSON.parse(e), t);
		} catch {}
	}
}
function Wd(e) {
	let t = [];
	return Ud(e, t), t;
}
//#endregion
//#region node_modules/ethers/lib.esm/wordlists/decode-owl.js
var Gd = " !#$%&'()*+,-./<=>?@[]^_`{|}~", Kd = /^[a-z]*$/i;
function qd(e, t) {
	let n = 97;
	return e.reduce((e, r) => (r === t ? n++ : r.match(Kd) ? e.push(String.fromCharCode(n) + r) : (n = 97, e.push(r)), e), []);
}
function Jd(e, t) {
	for (let n = 28; n >= 0; n--) e = e.split(Gd[n]).join(t.substring(2 * n, 2 * n + 2));
	let n = [], r = e.replace(/(:|([0-9])|([A-Z][a-z]*))/g, (e, t, r, i) => {
		if (r) for (let e = parseInt(r); e >= 0; e--) n.push(";");
		else n.push(t.toLowerCase());
		return "";
	});
	/* c8 ignore start */
	if (r) throw Error(`leftovers: ${JSON.stringify(r)}`);
	/* c8 ignore stop */
	return qd(qd(n, ";"), ":");
}
function Yd(e) {
	return d(e[0] === "0", "unsupported auwl data", "data", e), Jd(e.substring(59), e.substring(1, 59));
}
//#endregion
//#region node_modules/ethers/lib.esm/wordlists/wordlist.js
var Xd = class {
	locale;
	constructor(e) {
		a(this, { locale: e });
	}
	split(e) {
		return e.toLowerCase().split(/\s+/g);
	}
	join(e) {
		return e.join(" ");
	}
}, Zd = class extends Xd {
	#e;
	#t;
	constructor(e, t, n) {
		super(e), this.#e = t, this.#t = n, this.#n = null;
	}
	get _data() {
		return this.#e;
	}
	_decodeWords() {
		return Yd(this.#e);
	}
	#n;
	#r() {
		if (this.#n == null) {
			let e = this._decodeWords();
			/* c8 ignore start */
			if (go(e.join("\n") + "\n") !== this.#t) throw Error(`BIP39 Wordlist for ${this.locale} FAILED`);
			/* c8 ignore stop */
			this.#n = e;
		}
		return this.#n;
	}
	getWord(e) {
		let t = this.#r();
		return d(e >= 0 && e < t.length, `invalid word index: ${e}`, "index", e), t[e];
	}
	getWordIndex(e) {
		return this.#r().indexOf(e);
	}
}, Qd = "0erleonalorenseinceregesticitStanvetearctssi#ch2Athck&tneLl0And#Il.yLeOutO=S|S%b/ra@SurdU'0Ce[Cid|CountCu'Hie=IdOu,-Qui*Ro[TT]T%T*[Tu$0AptDD-tD*[Ju,M.UltV<)Vi)0Rob-0FairF%dRaid0A(EEntRee0Ead0MRRp%tS!_rmBumCoholErtI&LLeyLowMo,O}PhaReadySoT Ways0A>urAz(gOngOuntU'd0Aly,Ch%Ci|G G!GryIm$K!Noun)Nu$O` Sw T&naTiqueXietyY1ArtOlogyPe?P!Pro=Ril1ChCt-EaEnaGueMMedM%MyOundR<+Re,Ri=RowTTefa@Ti,Tw%k0KPe@SaultSetSi,SumeThma0H!>OmTa{T&dT.udeTra@0Ct]D.Gu,NtTh%ToTumn0Era+OcadoOid0AkeA*AyEsomeFulKw?d0Is:ByChel%C#D+GL<)Lc#y~MbooN<aNn RRelyRga(R*lSeS-SketTt!3A^AnAutyCau'ComeEfF%eG(Ha=H(dLie=LowLtN^Nef./TrayTt Twe&Y#d3Cyc!DKeNdOlogyRdR`Tt _{AdeAmeAnketA,EakE[IndOodO[omOu'UeUrUsh_rdAtDyIlMbNeNusOkO,Rd R(gRrowSsTtomUn)XY_{etA(AndA[A=EadEezeI{Id+IefIghtIngIskOccoliOk&OnzeOomO` OwnUsh2Bb!DdyD+tFf$oIldLbLkL!tNd!Nk Rd&Rg R,SS(e[SyTt Y Zz:Bba+B(B!CtusGeKe~LmM aMpNN$N)lNdyNn#NoeNvasNy#Pab!P.$Pta(RRb#RdRgoRpetRryRtSeShS(o/!Su$TT$ogT^Teg%yTt!UghtU'Ut]Ve3Il(gL yM|NsusNturyRe$Rta(_irAlkAmp]An+AosApt Ar+A'AtEapE{Ee'EfErryE,I{&IefIldIm}yOi)Oo'R#-U{!UnkUrn0G?Nnam#Rc!Tiz&TyVil_imApArifyAwAyE<ErkEv I{I|IffImbIn-IpO{OgO'O`OudOwnUbUmpU, Ut^_^A,C#utDeFfeeIlInL!@L%LumnMb(eMeMf%tM-Mm#Mp<yNc tNdu@NfirmNg*[N}@Nsid NtrolNv()OkOlPp PyR$ReRnR*@/Tt#U^UntryUp!Ur'Us(V Yo>_{Ad!AftAmA}AshAt AwlAzyEamEd.EekEwI{etImeIspIt-OpO[Ou^OwdUci$UelUi'Umb!Un^UshYY,$2BeLtu*PPbo?dRiousRr|Rta(R=Sh]/omTe3C!:DMa+MpN)Ng R(gShUght WnY3AlBa>BrisCadeCemb CideCl(eC%a>C*a'ErF&'F(eFyG*eLayLiv M<dMi'Ni$Nti,NyP?tP&dPos.P`PutyRi=ScribeS tSignSkSpair/royTailTe@VelopVi)Vo>3AgramAlAm#dAryCeE'lEtFf G.$Gn.yLemmaNn NosaurRe@RtSag*eScov Sea'ShSmi[S%d Splay/<)V tVideV%)Zzy5Ct%Cum|G~Lph(Ma(Na>NkeyN%OrSeUb!Ve_ftAg#AmaA,-AwEamE[IftIllInkIpI=OpUmY2CkMbNeR(g/T^Ty1Arf1Nam-:G G!RlyRnR`Sily/Sy1HoOlogyOnomy0GeItUca>1F%t0G1GhtTh 2BowD E@r-Eg<tEm|Eph<tEvat%I>Se0B?kBodyBra)Er+Ot]PloyPow Pty0Ab!A@DD![D%'EmyErgyF%)Ga+G(eH<)JoyLi,OughR-hRollSu*T Ti*TryVelope1Isode0U$Uip0AA'OdeOs]R%Upt0CapeSayS&)Ta>0Ern$H-s1Id&)IlOkeOl=1A@Amp!Ce[Ch<+C.eCludeCu'Ecu>Erci'Hau,Hib.I!I,ItOt-P<dPe@Pi*Pla(Po'P*[T&dTra0EEbrow:Br-CeCultyDeIntI`~L'MeMilyMousNNcyNtasyRmSh]TT$Th TigueUltV%.e3Atu*Bru?yD $EEdElMa!N)/iv$T^V W3B Ct]EldGu*LeLmLt N$NdNeNg NishReRmR,Sc$ShTT}[X_gAmeAshAtAv%EeIghtIpOatO{O%Ow UidUshY_mCusGIlLd~owOdOtR)Re,R+tRkRtu}RumRw?dSsil/ UndX_gi!AmeEqu|EshI&dIn+OgOntO,OwnOz&U.2ElNNnyRna)RyTu*:D+tInLaxy~ yMePRa+Rba+Rd&Rl-Rm|SSpTeTh U+Ze3N $NiusN*Nt!Nu(e/u*2O,0AntFtGg!Ng RaffeRlVe_dAn)A*A[IdeImp'ObeOomOryO=OwUe_tDde[LdOdO'RillaSpelSsipV nWn_bA)A(AntApeA[Av.yEatE&IdIefItOc yOupOwUnt_rdE[IdeIltIt?N3M:B.IrLfMm M, NdPpyRb%RdRshR=,TVeWkZ?d3AdAl`ArtAvyD+hogIght~oLmetLpNRo3Dd&Gh~NtPRe/%y5BbyCkeyLdLeLiday~owMeNeyOdPeRnRr%R'Sp.$/TelUrV 5BGeM<Mb!M%Nd*dNgryNtRd!RryRtSb<d3Brid:1EOn0EaEntifyLe2N%e4LLeg$L}[0A+Ita>M&'Mu}Pa@Po'Pro=Pul'0ChCludeComeC*a'DexD-a>Do%Du,ryF<tFl-tF%mHa!H .Iti$Je@JuryMa>N Noc|PutQuiryS<eSe@SideSpi*/$lTa@T e,ToVe,V.eVol=3On0L<dOla>Sue0Em1Ory:CketGu?RZz3AlousAns~yWel9BInKeUr}yY5D+I)MpNg!Ni%Nk/:Ng?oo3EnEpT^upY3CkDD}yNdNgdomSsTT^&TeTt&Wi4EeIfeO{Ow:BBelB%Dd DyKeMpNgua+PtopR+T T(UghUndryVaWWnWsu.Y Zy3Ad AfArnA=Ctu*FtGG$G&dIsu*M#NdNg`NsOp?dSs#Tt Vel3ArB tyBr?yC&'FeFtGhtKeMbM.NkOnQuid/Tt!VeZ?d5AdAnB, C$CkG-NelyNgOpTt yUdUn+VeY$5CkyGga+Mb N?N^Xury3R-s:Ch(eDG-G}tIdIlInJ%KeMm$NNa+Nda>NgoNs]Nu$P!Rb!R^Rg(R(eRketRria+SkSs/ T^T i$ThTrixTt XimumZe3AdowAnAsu*AtCh<-D$DiaLodyLtMb M%yNt]NuRcyR+R.RryShSsa+T$Thod3Dd!DnightLk~]M-NdNimumN%Nu>Rac!Rr%S ySs/akeXXedXtu*5Bi!DelDifyMM|N.%NkeyN, N`OnR$ReRn(gSqu.oTh T]T%Unta(U'VeVie5ChFf(LeLtiplySc!SeumShroomS-/Tu$3Self/ yTh:I=MePk(Rrow/yT]Tu*3ArCkEdGati=G!@I` PhewR=/TTw%kUtr$V WsXt3CeGht5B!I'M(eeOd!Rm$R`SeTab!TeTh(gTi)VelW5C!?Mb R'T:K0EyJe@Li+Scu*S =Ta(Vious0CurE<Tob 0Or1FF Fi)T&2L1Ay0DI=Ymp-0It0CeEI#L(eLy1EnEraIn]Po'T]1An+B.Ch?dD D(?yG<I|Ig($Ph<0Tr-h0H 0Tdo%T TputTside0AlEnEr0NN 0Yg&0/ 0O}:CtDd!GeIrLa)LmNdaNelN-N` P RadeR|RkRrotRtySsT^ThTi|TrolTt nU'VeYm|3A)AnutArAs<tL-<NN$tyNcilOp!Pp Rfe@Rm.Rs#T2O}OtoRa'Ys-$0AnoCn-Ctu*E)GGe#~LotNkO} Pe/olT^Zza_)A}tA,-A>AyEa'Ed+U{UgUn+2EmEtIntL?LeLi)NdNyOlPul?Rt]S.]Ssib!/TatoTt yV tyWd W _@i)Ai'Ed-tEf Epa*Es|EttyEv|I)IdeIm?yIntI%.yIs#Iva>IzeOb!mO)[Odu)Of.OgramOje@Omo>OofOp tyOsp O>@OudOvide2Bl-Dd(g~LpL'Mpk(N^PilPpyR^a'R.yRpo'R'ShTZz!3Ramid:99Al.yAntumArt E,]I{ItIzO>:Bb.Cco#CeCkD?DioIlInI'~yMpN^NdomN+PidReTeTh V&WZ%3AdyAlAs#BelBuildC$lCei=CipeC%dCyc!Du)F!@F%mFu'G]G*tGul?Je@LaxLea'LiefLyMa(Memb M(dMo=Nd NewNtOp&PairPeatPla)P%tQui*ScueSemb!Si,Sour)Sp#'SultTi*T*atTurnUn]Ve$ViewW?d2Y`m0BBb#CeChDeD+F!GhtGidNgOtPp!SkTu$V$V 5AdA,BotBu,CketM<)OfOkieOmSeTa>UghUndU>Y$5Bb DeGLeNNwayR$:DDd!D}[FeIlLadLm#L#LtLu>MeMp!NdTisfyToshiU)Usa+VeY1A!AnA*Att E}HemeHoolI&)I[%sOrp]OutRapRe&RiptRub1AAr^As#AtC#dC*tCt]Cur.yEdEkGm|Le@~M(?Ni%N'Nt&)RiesRvi)Ss]Tt!TupV&_dowAftAllowA*EdEllEriffIeldIftI}IpIv O{OeOotOpOrtOuld O=RimpRugUff!Y0Bl(gCkDeE+GhtGnL|Lk~yLv Mil?Mp!N)NgR&/ Tua>XZe1A>Et^IIllInIrtUll0AbAmEepEnd I)IdeIghtImOg<OtOwUsh0AllArtI!OkeOo`0A{AkeApIffOw0ApCc Ci$CkDaFtL?Ldi LidLut]L=Me#eNgOnRryRtUlUndUpUr)U`0A)A*Ati$AwnEakEci$EedEllEndH eI)Id IkeInIr.L.OilOns%O#OrtOtRayReadR(gY0Ua*UeezeUir*l_b!AdiumAffA+AirsAmpAndArtA>AyEakEelEmEpE*oI{IllIngO{Oma^O}OolOryO=Ra>gyReetRikeR#gRugg!Ud|UffUmb!Y!0Bje@Bm.BwayC)[ChDd&Ff G?G+,ItMm NNnyN'tP PplyP*meReRfa)R+Rpri'RroundR=ySpe@/a(1AllowAmpApArmE?EetIftImIngIt^Ord1MbolMptomRup/em:B!Ck!GIlL|LkNkPeR+tSk/eTtooXi3A^Am~NN<tNnisNtRm/Xt_nkAtEmeEnE%yE*EyIngIsOughtReeRi=RowUmbUnd 0CketDeG LtMb MeNyPRedSsueT!5A,BaccoDayDdl EGe` I!tK&MatoM%rowNeNgueNightOlO`PP-Pp!R^RnadoRtoi'SsT$Uri,W?dW WnY_{AdeAff-Ag-A(Ansf ApAshA=lAyEatEeEndI$IbeI{Igg ImIpOphyOub!U{UeUlyUmpetU,U`Y2BeIt]Mb!NaN}lRkeyRnRt!1El=EntyI)InI,O1PeP-$:5Ly5B*lla0Ab!Awa*C!Cov D DoFairFoldHappyIf%mIqueItIv 'KnownLo{TilUsu$Veil1Da>GradeHoldOnP Set1B<Ge0A+EEdEfulE![U$0Il.y:C<tCuumGueLidL!yL=NNishP%Rious/Ult3H-!L=tNd%Ntu*NueRbRifyRs]RyS'lT <3Ab!Br<tCiousCt%yDeoEw~a+Nta+Ol(Rtu$RusSaS.Su$T$Vid5C$I)IdLc<oLumeTeYa+:GeG#ItLk~LnutNtRfa*RmRri%ShSp/eT VeY3Al`Ap#ArA'lA` BDd(gEk&dIrdLcome/T_!AtEatEelEnE*IpIsp 0DeD`FeLd~NNdowNeNgNkNn Nt ReSdomSeShT}[5LfM<Nd OdOlRdRkRldRryR`_pE{E,!I,I>Ong::Rd3Ar~ow9UUngU`:3BraRo9NeO", $d = "0x3c8acc1e7b08d8e76f9fda015ef48dc8c710a73cb7e0f77b2c18a9b5a7adde60", ef = null, tf = class e extends Zd {
	constructor() {
		super("en", Qd, $d);
	}
	static wordlist() {
		return ef ??= new e(), ef;
	}
};
//#endregion
//#region node_modules/ethers/lib.esm/wallet/mnemonic.js
function nf(e) {
	return (1 << e) - 1 << 8 - e & 255;
}
function rf(e) {
	return (1 << e) - 1 & 255;
}
function af(e, t) {
	m("NFKD"), t ??= tf.wordlist();
	let n = t.split(e);
	d(n.length % 3 == 0 && n.length >= 12 && n.length <= 24, "invalid mnemonic length", "mnemonic", "[ REDACTED ]");
	let r = new Uint8Array(Math.ceil(11 * n.length / 8)), i = 0;
	for (let e = 0; e < n.length; e++) {
		let a = t.getWordIndex(n[e].normalize("NFKD"));
		d(a >= 0, `invalid mnemonic word at index ${e}`, "mnemonic", "[ REDACTED ]");
		for (let e = 0; e < 11; e++) a & 1 << 10 - e && (r[i >> 3] |= 1 << 7 - i % 8), i++;
	}
	let a = 32 * n.length / 3, o = nf(n.length / 3);
	return d((_(_r(r.slice(0, a / 8)))[0] & o) === (r[r.length - 1] & o), "invalid mnemonic checksum", "mnemonic", "[ REDACTED ]"), S(r.slice(0, a / 8));
}
function of(e, t) {
	d(e.length % 4 == 0 && e.length >= 16 && e.length <= 32, "invalid entropy size", "entropy", "[ REDACTED ]"), t ??= tf.wordlist();
	let n = [0], r = 11;
	for (let t = 0; t < e.length; t++) r > 8 ? (n[n.length - 1] <<= 8, n[n.length - 1] |= e[t], r -= 8) : (n[n.length - 1] <<= r, n[n.length - 1] |= e[t] >> 8 - r, n.push(e[t] & rf(8 - r)), r += 3);
	let i = e.length / 4, a = parseInt(_r(e).substring(2, 4), 16) & nf(i);
	return n[n.length - 1] <<= i, n[n.length - 1] |= a >> 8 - i, t.join(n.map((e) => t.getWord(e)));
}
var sf = {}, cf = class e {
	phrase;
	password;
	wordlist;
	entropy;
	constructor(e, t, n, r, i) {
		r ??= "", i ??= tf.wordlist(), h(e, sf, "Mnemonic"), a(this, {
			phrase: n,
			password: r,
			wordlist: i,
			entropy: t
		});
	}
	computeSeed() {
		let e = H("mnemonic" + this.password, "NFKD");
		return Yn(H(this.phrase, "NFKD"), e, 2048, 64, "sha512");
	}
	static fromPhrase(t, n, r) {
		let i = af(t, r);
		return t = of(_(i), r), new e(sf, i, t, n, r);
	}
	static fromEntropy(t, n, r) {
		let i = _(t, "entropy"), a = of(i, r);
		return new e(sf, S(i), a, n, r);
	}
	static entropyToPhrase(e, t) {
		return of(_(e, "entropy"), t);
	}
	static phraseToEntropy(e, t) {
		return af(e, t);
	}
	static isValidMnemonic(e, t) {
		try {
			return af(e, t), !0;
		} catch {}
		return !1;
	}
};
//#endregion
//#region node_modules/@noble/ciphers/utils.js
function lf(e) {
	return e instanceof Uint8Array || ArrayBuffer.isView(e) && e.constructor.name === "Uint8Array" && "BYTES_PER_ELEMENT" in e && e.BYTES_PER_ELEMENT === 1;
}
var uf = (e) => e ? `"${e}" ` : "";
function df(e, t = "") {
	if (typeof e != "boolean") throw TypeError(uf(t) + "expected boolean, got type=" + typeof e);
	return e;
}
function ff(e, t = "") {
	if (typeof e != "number") throw TypeError(uf(t) + "expected number, got " + typeof e);
	if (!Number.isSafeInteger(e) || e < 0) throw RangeError(uf(t) + "expected integer >= 0, got " + e);
	return e;
}
function pf(e, t, n = "") {
	if (lf(e) && (t === void 0 || e.length === t)) return e;
	t !== void 0 && ff(t, "length");
	let r = lf(e), i = t === void 0 ? "" : ` of length ${t}`, a = r ? `length=${e.length}` : `type=${typeof e}`, o = uf(n) + "expected Uint8Array" + i + ", got " + a;
	throw r ? RangeError(o) : TypeError(o);
}
var mf = (e, t) => {
	if (typeof e != "object" || !e || Array.isArray(e)) throw TypeError(t === "object" ? "expected valid options object" : `"${t}" expected object, got type=${typeof e}`);
};
function hf(e, t = !0) {
	if (e.destroyed) throw Error("hash was destroyed");
	if (t && e.finished) throw Error("digest() was already called");
}
function gf(e, t) {
	pf(e, void 0, "output");
	let n = t.outputLen;
	if (!(e.length >= n)) throw RangeError("\"output\" expected length >= " + n);
}
function _f(e) {
	return new Uint8Array(e.buffer, e.byteOffset, e.byteLength);
}
function vf(e) {
	return new Uint32Array(e.buffer, e.byteOffset, Math.floor(e.byteLength / 4));
}
function yf(...e) {
	for (let t = 0; t < e.length; t++) e[t].fill(0);
}
function bf(e) {
	return new DataView(e.buffer, e.byteOffset, e.byteLength);
}
var xf = new Uint8Array(new Uint32Array([287454020]).buffer)[0] === 68;
function Sf(e) {
	return e << 24 & 4278190080 | e << 8 & 16711680 | e >>> 8 & 65280 | e >>> 24 & 255;
}
var Cf = xf ? (e) => e : (e) => Sf(e) >>> 0;
function wf(e) {
	for (let t = 0; t < e.length; t++) e[t] = Sf(e[t]);
	return e;
}
var Tf = xf ? (e) => e : wf;
function Ef(e, t) {
	return !e.byteLength || !t.byteLength ? !1 : e.buffer === t.buffer && e.byteOffset < t.byteOffset + t.byteLength && t.byteOffset < e.byteOffset + e.byteLength;
}
function Df(e, t) {
	if (Ef(e, t) && e.byteOffset < t.byteOffset) throw Error("complex overlap of input and output is not supported");
}
function Of(e, t) {
	return mf(e, "defaults"), mf(t, "opts"), Object.assign(e, t);
}
function kf(e, t) {
	if (e = pf(e), t = pf(t), e.length !== t.length) return !1;
	let n = 0;
	for (let r = 0; r < e.length; r++) n |= e[r] ^ t[r];
	return n === 0;
}
function Af(e, t, n) {
	let r = t, i = n || (() => []), a = (e, t) => r(t, ...i(e)).update(e).digest(), o = r(new Uint8Array(e), ...i(/* @__PURE__ */ new Uint8Array()));
	return a.outputLen = o.outputLen, a.blockLen = o.blockLen, a.create = (e, ...t) => r(e, ...t), a;
}
var jf = (e, t) => {
	function n(n, ...r) {
		if (pf(n, void 0, "key"), e.nonceLength !== void 0) {
			let t = r[0];
			pf(t, e.varSizeNonce ? void 0 : e.nonceLength, "nonce");
		}
		let i = e.tagLength, a = e.nonceLength === void 0 ? 0 : 1;
		if (!e.withAAD) {
			for (let e = a; e < r.length; e++) if (lf(r[e])) throw Error("AAD not supported");
		}
		e.withAAD && r[a] !== void 0 && pf(r[a], void 0, "AAD");
		let o = t(n, ...r), s = (e, t) => {
			if (t !== void 0) {
				if (e !== 2) throw Error("cipher output not supported");
				pf(t, void 0, "output");
			}
		}, c = !1;
		return {
			encrypt(e, t) {
				if (c) throw Error("cannot encrypt() twice with same key + nonce");
				return c = !0, pf(e, void 0, "data"), s(o.encrypt.length, t), o.encrypt(e, t);
			},
			decrypt(e, t) {
				if (pf(e, void 0, "data"), i && e.length < i) throw Error("\"ciphertext\" expected length >= tagLength=" + i);
				return s(o.decrypt.length, t), o.decrypt(e, t);
			}
		};
	}
	return Object.assign(n, e), n;
};
function Mf(e, t, n = !0) {
	if (t === void 0) return new Uint8Array(e);
	if (pf(t, e, "output"), n && !Pf(t)) throw Error("invalid output, must be aligned");
	return t;
}
function Nf(e, t, n) {
	ff(e), ff(t), df(n);
	let r = /* @__PURE__ */ new Uint8Array(16), i = bf(r);
	return i.setBigUint64(0, BigInt(t), n), i.setBigUint64(8, BigInt(e), n), r;
}
function Pf(e) {
	return e.byteOffset % 4 == 0;
}
function Ff(e) {
	return Uint8Array.from(pf(e));
}
//#endregion
//#region node_modules/@noble/ciphers/aes.js
var If = 16, Lf = 4, Rf = 283;
function zf(e) {
	if (![
		16,
		24,
		32
	].includes(e.length)) throw Error("\"aes key\" expected Uint8Array of length 16/24/32, got length=" + e.length);
}
function Bf(e) {
	return e << 1 ^ Rf & -(e >> 7);
}
function Vf(e, t) {
	let n = 0;
	for (; t > 0; t >>= 1) n ^= e & -(t & 1), e = Bf(e);
	return n;
}
var Hf = /* @__PURE__ */ (() => {
	let e = /* @__PURE__ */ new Uint8Array(256);
	for (let t = 0, n = 1; t < 256; t++, n ^= Bf(n)) e[t] = n;
	let t = /* @__PURE__ */ new Uint8Array(256);
	t[0] = 99;
	for (let n = 0; n < 255; n++) {
		let r = e[255 - n];
		r |= r << 8, t[e[n]] = (r ^ r >> 4 ^ r >> 5 ^ r >> 6 ^ r >> 7 ^ 99) & 255;
	}
	return yf(e), t;
})(), Uf = (e) => e << 24 | e >>> 8, Wf = (e) => e << 8 | e >>> 24;
function Gf(e, t) {
	if (e.length !== 256) throw Error("wrong sbox length");
	let n = (/* @__PURE__ */ new Uint32Array(256)).map((n, r) => t(e[r])), r = n.map(Wf), i = r.map(Wf), a = i.map(Wf), o = /* @__PURE__ */ new Uint32Array(65536), s = /* @__PURE__ */ new Uint32Array(65536), c = /* @__PURE__ */ new Uint16Array(65536);
	for (let t = 0; t < 256; t++) for (let l = 0; l < 256; l++) {
		let u = t * 256 + l;
		o[u] = n[t] ^ r[l], s[u] = i[t] ^ a[l], c[u] = e[t] << 8 | e[l];
	}
	return {
		sbox: e,
		sbox2: c,
		T0: n,
		T1: r,
		T2: i,
		T3: a,
		T01: o,
		T23: s
	};
}
var Kf = /* @__PURE__ */ Gf(Hf, (e) => Vf(e, 3) << 24 | e << 16 | e << 8 | Vf(e, 2)), qf = /* @__PURE__ */ (() => {
	let e = /* @__PURE__ */ new Uint8Array(16);
	for (let t = 0, n = 1; t < 16; t++, n = Bf(n)) e[t] = n;
	return e;
})();
function Jf(e) {
	pf(e);
	let t = e.length;
	zf(e);
	let { sbox2: n } = Kf, r = [];
	(!xf || !Pf(e)) && r.push(e = Ff(e));
	let i = Tf(vf(e)), a = i.length, o = (e) => Xf(n, e, e, e, e), s = new Uint32Array(t + 28);
	s.set(i);
	for (let e = a; e < s.length; e++) {
		let t = s[e - 1];
		e % a === 0 ? t = o(Uf(t)) ^ qf[e / a - 1] : a > 6 && e % a === 4 && (t = o(t)), s[e] = s[e - a] ^ t;
	}
	return yf(...r), s;
}
function Yf(e, t, n, r, i, a) {
	return e[n << 8 & 65280 | r >>> 8 & 255] ^ t[i >>> 8 & 65280 | a >>> 24 & 255];
}
function Xf(e, t, n, r, i) {
	return e[t & 255 | n & 65280] | e[r >>> 16 & 255 | i >>> 16 & 65280] << 16;
}
function Zf(e, t, n, r, i) {
	let { sbox2: a, T01: o, T23: s } = Kf, c = 0;
	t ^= e[c++], n ^= e[c++], r ^= e[c++], i ^= e[c++];
	let l = e.length / 4 - 2;
	for (let a = 0; a < l; a++) {
		let a = e[c++] ^ Yf(o, s, t, n, r, i), l = e[c++] ^ Yf(o, s, n, r, i, t), u = e[c++] ^ Yf(o, s, r, i, t, n), d = e[c++] ^ Yf(o, s, i, t, n, r);
		t = a, n = l, r = u, i = d;
	}
	return {
		s0: e[c++] ^ Xf(a, t, n, r, i),
		s1: e[c++] ^ Xf(a, n, r, i, t),
		s2: e[c++] ^ Xf(a, r, i, t, n),
		s3: e[c++] ^ Xf(a, i, t, n, r)
	};
}
function Qf(e, t, n, r) {
	pf(t, If, "nonce"), pf(n);
	let i = n.length;
	r = Mf(i, r), Df(n, r);
	let a = t, o = vf(a), s = vf(n), c = vf(r);
	for (let t = 0; t + 4 <= s.length; t += 4) {
		let { s0: n, s1: r, s2: i, s3: l } = Zf(e, Cf(o[0]), Cf(o[1]), Cf(o[2]), Cf(o[3]));
		c[t + 0] = s[t + 0] ^ Cf(n), c[t + 1] = s[t + 1] ^ Cf(r), c[t + 2] = s[t + 2] ^ Cf(i), c[t + 3] = s[t + 3] ^ Cf(l);
		for (let e = 15, t = 1; e >= 0; e--) t = t + a[e] | 0, a[e] = t & 255, t >>>= 8;
	}
	let l = If * Math.floor(s.length / Lf);
	if (l < i) {
		let { s0: t, s1: a, s2: s, s3: c } = Zf(e, Cf(o[0]), Cf(o[1]), Cf(o[2]), Cf(o[3])), u = new Uint32Array([
			t,
			a,
			s,
			c
		]);
		Tf(u);
		let d = _f(u);
		for (let e = l, t = 0; e < i; e++, t++) r[e] = n[e] ^ d[t];
		yf(u);
	}
	return r;
}
var $f = /* @__PURE__ */ jf({
	blockSize: 16,
	nonceLength: 16
}, function(e, t) {
	function n(n, r) {
		if (pf(n), r !== void 0 && (pf(r), !Pf(r))) throw Error("unaligned destination");
		let i = Jf(e), a = Ff(t), o = [i, a];
		Pf(n) || o.push(n = Ff(n));
		let s = Qf(i, a, n, r);
		return yf(...o), s;
	}
	return {
		encrypt: (e, t) => n(e, t),
		decrypt: (e, t) => n(e, t)
	};
});
//#endregion
//#region node_modules/@noble/post-quantum/node_modules/@noble/hashes/utils.js
function ep(e) {
	return e instanceof Uint8Array || ArrayBuffer.isView(e) && e.constructor.name === "Uint8Array" && "BYTES_PER_ELEMENT" in e && e.BYTES_PER_ELEMENT === 1;
}
var tp = (e) => e ? `"${e}" ` : "";
function np(e, t = "") {
	if (typeof e != "number") throw TypeError(tp(t) + "expected number, got " + typeof e);
	if (!Number.isSafeInteger(e) || e < 0) throw RangeError(tp(t) + "expected integer >= 0, got " + e);
	return e;
}
function rp(e, t = "") {
	if (typeof e != "boolean") throw TypeError(tp(t) + "expected boolean, got type=" + typeof e);
	return e;
}
function ip(e, t, n = "") {
	if (ep(e) && (t === void 0 || e.length === t)) return e;
	t !== void 0 && np(t, "length");
	let r = ep(e), i = t === void 0 ? "" : ` of length ${t}`, a = r ? `length=${e.length}` : `type=${typeof e}`, o = tp(n) + "expected Uint8Array" + i + ", got " + a;
	throw r ? RangeError(o) : TypeError(o);
}
function ap(e) {
	if (typeof e != "function" || typeof e.create != "function") throw TypeError("expected hash wrapped by utils.createHasher");
	if (np(e.outputLen), np(e.blockLen), e.outputLen < 1 || e.blockLen < 1) throw Error("hash blockLen / outputLen must be >= 1");
}
var op = (e, t) => {
	if (typeof e != "object" || !e || Array.isArray(e)) throw TypeError((t === "object" ? "" : `"${t}" `) + "expected object, got type=" + typeof e);
};
function sp(e, t = !0) {
	if (e.destroyed) throw Error("hash was destroyed");
	if (t && e.finished) throw Error("digest() was already called");
}
function cp(e, t) {
	ip(e, void 0, "output");
	let n = t.outputLen;
	if (!(e.length >= n)) throw RangeError("\"output\" expected length >= " + n);
}
function lp(e) {
	return new Uint32Array(e.buffer, e.byteOffset, Math.floor(e.byteLength / 4));
}
function up(...e) {
	for (let t = 0; t < e.length; t++) e[t].fill(0);
}
var dp = new Uint8Array(new Uint32Array([287454020]).buffer)[0] === 68;
function fp(e) {
	return e << 24 & 4278190080 | e << 8 & 16711680 | e >>> 8 & 65280 | e >>> 24 & 255;
}
function pp(e) {
	for (let t = 0; t < e.length; t++) e[t] = fp(e[t]);
	return e;
}
var mp = dp ? (e) => e : pp;
function hp(...e) {
	let t = 0;
	for (let n = 0; n < e.length; n++) {
		let r = e[n];
		ip(r), t += r.length;
	}
	let n = new Uint8Array(t);
	for (let t = 0, r = 0; t < e.length; t++) {
		let i = e[t];
		n.set(i, r), r += i.length;
	}
	return n;
}
function gp(e, t, n = "opts") {
	return op(e, "defaults"), t !== void 0 && op(t, n), Object.assign(e, t);
}
function _p(e, t = {}) {
	if (typeof e != "function") throw TypeError("\"hashCons\" expected function, got type=" + typeof e);
	t = gp({}, t, "info");
	let n = (t, n) => e(n).update(t).digest(), r = e(void 0);
	return n.outputLen = r.outputLen, n.blockLen = r.blockLen, n.canXOF = r.canXOF, n.create = (t) => e(t), Object.assign(n, t), Object.freeze(n);
}
function vp(e = 32) {
	np(e, "bytesLength");
	let t = typeof globalThis == "object" ? globalThis.crypto : null;
	if (typeof t?.getRandomValues != "function") throw Error("crypto.getRandomValues must be defined");
	if (e > 65536) throw RangeError(`"bytesLength" expected <= 65536, got ${e}`);
	return t.getRandomValues(new Uint8Array(e));
}
var yp = (e) => ({ oid: Uint8Array.from([
	6,
	9,
	96,
	134,
	72,
	1,
	101,
	3,
	4,
	2,
	e
]) });
//#endregion
//#region node_modules/@noble/post-quantum/node_modules/@noble/curves/utils.js
function bp(e, t = "object") {
	if (typeof e != "object" || !e || Array.isArray(e)) throw TypeError(t === "object" ? "expected valid options object" : `"${t}" expected object, got type=${typeof e}`);
	return e;
}
var xp = (e) => e ? `"${e}" ` : "";
function Sp(e, t = "") {
	if (typeof e != "boolean") throw TypeError(xp(t) + "expected boolean, got type=" + typeof e);
	return e;
}
function Cp(e, t = {}, n = {}, r = "object") {
	bp(e, r), bp(t, "fields"), bp(n, "optFields");
	function i(t, n, i) {
		let a = r === "object" ? `param "${String(t)}"` : `"${r}.${String(t)}"`, o = e[t];
		if (!Object.hasOwn(e, t) && (i ? o !== void 0 : n !== "function")) throw TypeError(`${a} is invalid: expected own property`);
		if (i && o === void 0) return;
		let s = typeof o;
		if (s !== n || o === null) throw TypeError(`${a} is invalid: expected ${n}, got ${s}`);
	}
	let a = (e, t) => Object.entries(e).forEach(([e, n]) => i(e, n, t));
	a(t, !1), a(n, !0);
}
//#endregion
//#region node_modules/@noble/post-quantum/node_modules/@noble/hashes/_u64.js
var wp = /* @__PURE__ */ BigInt(2 ** 32 - 1), Tp = /* @__PURE__ */ BigInt(32);
function Ep(e, t = !1) {
	return t ? {
		h: Number(e & wp),
		l: Number(e >> Tp & wp)
	} : {
		h: Number(e >> Tp & wp) | 0,
		l: Number(e & wp) | 0
	};
}
function Dp(e, t = !1) {
	let n = e.length, r = new Uint32Array(n), i = new Uint32Array(n);
	for (let a = 0; a < n; a++) {
		let { h: n, l: o } = Ep(e[a], t);
		[r[a], i[a]] = [n, o];
	}
	return [r, i];
}
//#endregion
//#region node_modules/@noble/post-quantum/node_modules/@noble/hashes/sha3.js
var Op = BigInt(0), kp = BigInt(1), Ap = BigInt(2), jp = BigInt(7), Mp = BigInt(256), Np = BigInt(113), Pp = [], Fp = [], Ip = [];
for (let e = 0, t = kp, n = 1, r = 0; e < 24; e++) {
	[n, r] = [r, (2 * n + 3 * r) % 5], Pp.push(2 * (5 * r + n)), Fp.push((e + 1) * (e + 2) / 2 % 64);
	let i = Op;
	for (let e = 0; e < 7; e++) t = (t << kp ^ (t >> jp) * Np) % Mp, t & Ap && (i ^= kp << (kp << BigInt(e)) - kp);
	Ip.push(i);
}
var Lp = Dp(Ip, !0), Rp = Lp[0], zp = Lp[1], Bp = (e, t, n) => e << n | t >>> 32 - n, Vp = (e, t, n) => t << n | e >>> 32 - n, Hp = (e, t, n) => t << n - 32 | e >>> 64 - n, Up = (e, t, n) => e << n - 32 | t >>> 64 - n, Wp = (e, t, n) => n > 32 ? Hp(e, t, n) : Bp(e, t, n), Gp = (e, t, n) => n > 32 ? Up(e, t, n) : Vp(e, t, n), Kp = /* @__PURE__ */ new Uint32Array(10);
function qp(e, t = 24) {
	if (!(e instanceof Uint32Array)) throw TypeError("\"s\" expected Uint32Array(50), got type=" + typeof e);
	if (e.length !== 50) throw RangeError("\"s\" expected Uint32Array(50), got length=" + e.length);
	if (np(t, "rounds"), t < 1 || t > 24) throw Error("\"rounds\" expected integer 1..24");
	for (let n = 24 - t; n < 24; n++) {
		for (let t = 0; t < 10; t++) Kp[t] = e[t] ^ e[t + 10] ^ e[t + 20] ^ e[t + 30] ^ e[t + 40];
		for (let t = 0; t < 10; t += 2) {
			let n = (t + 8) % 10, r = (t + 2) % 10, i = Kp[r], a = Kp[r + 1], o = Wp(i, a, 1) ^ Kp[n], s = Gp(i, a, 1) ^ Kp[n + 1];
			for (let n = 0; n < 50; n += 10) e[t + n] ^= o, e[t + n + 1] ^= s;
		}
		let t = e[2], r = e[3];
		for (let n = 0; n < 24; n++) {
			let i = Fp[n], a = Wp(t, r, i), o = Gp(t, r, i), s = Pp[n];
			t = e[s], r = e[s + 1], e[s] = a, e[s + 1] = o;
		}
		for (let t = 0; t < 50; t += 10) {
			let n = e[t], r = e[t + 1], i = e[t + 2], a = e[t + 3];
			e[t] ^= ~e[t + 2] & e[t + 4], e[t + 1] ^= ~e[t + 3] & e[t + 5], e[t + 2] ^= ~e[t + 4] & e[t + 6], e[t + 3] ^= ~e[t + 5] & e[t + 7], e[t + 4] ^= ~e[t + 6] & e[t + 8], e[t + 5] ^= ~e[t + 7] & e[t + 9], e[t + 6] ^= ~e[t + 8] & n, e[t + 7] ^= ~e[t + 9] & r, e[t + 8] ^= ~n & i, e[t + 9] ^= ~r & a;
		}
		e[0] ^= Rp[n], e[1] ^= zp[n];
	}
	up(Kp);
}
var Jp = class e {
	state;
	pos = 0;
	posOut = 0;
	finished = !1;
	state32;
	destroyed = !1;
	blockLen;
	suffix;
	outputLen;
	canXOF;
	enableXOF = !1;
	rounds;
	constructor(e, t, n, r = !1, i = 24) {
		if (np(e, "blockLen"), np(t, "suffix"), np(i, "rounds"), rp(r, "enableXOF"), this.blockLen = e, this.suffix = t, this.outputLen = n, this.enableXOF = r, this.canXOF = r, this.rounds = i, np(n, "outputLen"), !(0 < e && e < 200)) throw Error("\"blockLen\" must be 1..199");
		this.state = /* @__PURE__ */ new Uint8Array(200), this.state32 = lp(this.state);
	}
	clone() {
		return this._cloneInto();
	}
	keccak() {
		mp(this.state32), qp(this.state32, this.rounds), mp(this.state32), this.posOut = 0, this.pos = 0;
	}
	update(e) {
		sp(this), ip(e);
		let { blockLen: t, state: n, state32: r } = this, i = e.length, a = t % 4 == 0 && e.byteOffset % 4 == 0, o = t / 4, s = a && i >= t ? lp(e) : void 0;
		for (let a = 0; a < i;) {
			if (s !== void 0 && this.pos === 0 && a % 4 == 0 && i - a >= t) {
				for (let e = 0, t = a / 4; e < o; e++) r[e] ^= s[t + e];
				a += t, this.pos = t, this.keccak();
				continue;
			}
			let c = Math.min(t - this.pos, i - a);
			for (let t = 0; t < c; t++) n[this.pos++] ^= e[a++];
			this.pos === t && this.keccak();
		}
		return this;
	}
	finish() {
		if (this.finished) return;
		this.finished = !0;
		let { state: e, suffix: t, pos: n, blockLen: r } = this;
		e[n] ^= t, t & 128 && n === r - 1 && this.keccak(), e[r - 1] ^= 128, this.keccak();
	}
	writeInto(e) {
		sp(this, !1), ip(e), this.finish();
		let t = this.state, { blockLen: n } = this;
		for (let r = 0, i = e.length; r < i;) {
			this.posOut >= n && this.keccak();
			let a = Math.min(n - this.posOut, i - r);
			e.set(t.subarray(this.posOut, this.posOut + a), r), this.posOut += a, r += a;
		}
		return e;
	}
	xofInto(e) {
		if (!this.enableXOF) throw Error("XOF is not enabled");
		return this.writeInto(e);
	}
	xof(e) {
		return np(e), this.xofInto(new Uint8Array(e));
	}
	digestInto(e) {
		if (cp(e, this), this.finished) throw Error("digest() was already called");
		this.writeInto(e.length === this.outputLen ? e : e.subarray(0, this.outputLen)), this.destroy();
	}
	digest() {
		let e = new Uint8Array(this.outputLen);
		return this.digestInto(e), e;
	}
	destroy() {
		this.destroyed = !0, up(this.state);
	}
	_cloneInto(t) {
		let { blockLen: n, suffix: r, outputLen: i, rounds: a, enableXOF: o } = this;
		return t ||= new e(n, r, i, o, a), t.blockLen = n, t.state32.set(this.state32), t.pos = this.pos, t.posOut = this.posOut, t.finished = this.finished, t.rounds = a, t.suffix = r, t.outputLen = i, t.enableXOF = o, t.canXOF = this.canXOF, t.destroyed = this.destroyed, t;
	}
}, Yp = (e, t, n, r = {}) => _p(() => new Jp(t, e, n), r), Xp = /* @__PURE__ */ Yp(6, 136, 32, /* @__PURE__ */ yp(8)), Zp = /* @__PURE__ */ Yp(6, 72, 64, /* @__PURE__ */ yp(10)), Qp = (e, t, n, r = {}) => _p((r = {}) => (r = gp({}, r), new Jp(t, e, r.dkLen === void 0 ? n : r.dkLen, !0)), r), $p = /* @__PURE__ */ Qp(31, 168, 16, /* @__PURE__ */ yp(11)), em = /* @__PURE__ */ Qp(31, 136, 32, /* @__PURE__ */ yp(12));
//#endregion
//#region node_modules/@noble/post-quantum/node_modules/@noble/curves/abstract/fft.js
function tm(e, t = "n") {
	if (typeof e != "number") throw TypeError(`wrong u32 integer "${t}": expected number, got type=${typeof e}`);
	if (!Number.isSafeInteger(e) || e < 0 || e > 4294967295) throw RangeError(`wrong u32 integer "${t}": expected 0..4294967295, got ${e}`);
	return e;
}
function nm(e) {
	return tm(e, "x"), !(e & e - 1) && e !== 0;
}
function rm(e, t) {
	if (tm(e), typeof t != "number") throw TypeError("\"bits\" expected number, got type=" + typeof t);
	if (!Number.isSafeInteger(t) || t < 0 || t > 32) throw Error(`expected integer 0 <= bits <= 32, got ${t}`);
	let n = 0;
	for (let r = 0; r < t; r++, e >>>= 1) n = n << 1 | e & 1;
	return n >>> 0;
}
function im(e) {
	return tm(e), 31 - Math.clz32(e);
}
function am(e) {
	if (!e || typeof e != "object" || typeof e.length != "number") throw TypeError("\"values\" expected array-like, got type=" + typeof e);
	let t = e.length;
	if (!nm(t)) throw Error("expected positive power-of-two length, got " + t);
	let n = im(t);
	for (let r = 0; r < t; r++) {
		let t = rm(r, n);
		if (r < t) {
			let n = e[r];
			e[r] = e[t], e[t] = n;
		}
	}
	return e;
}
var om = (e, t) => {
	Cp(t, {
		N: "number",
		roots: "object",
		dit: "boolean"
	}, {
		invertButterflies: "boolean",
		skipStages: "number",
		brp: "boolean"
	}, "coreOpts");
	let { N: n, roots: r, dit: i, invertButterflies: a = !1, skipStages: o = 0, brp: s = !0 } = t;
	tm(n, "coreOpts.N");
	let c = im(n);
	if (!nm(n)) throw Error("FFT: Polynomial size should be power of two");
	tm(o, "coreOpts.skipStages");
	let l = c === 0 ? 0 : c - 1;
	if (o > l) throw Error(`FFT: wrong skipStages: expected 0 <= skipStages <= ${l}`);
	if (r.length !== n) throw Error(`FFT: wrong roots length: expected ${n}, got ${r.length}`);
	let u = i !== a;
	return (t) => {
		if (t.length !== n) throw Error("FFT: wrong Polynomial length");
		i && s && am(t);
		for (let s = 0, l = 1; s < c - o; s++) {
			let d = i ? s + 1 + o : c - s, f = 1 << d, p = f >> 1, m = n >> d;
			for (let o = 0; o < n; o += f) for (let s = 0, c = l++; s < p; s++) {
				let l = a ? i ? n - c : c : s * m, d = o + s, f = o + s + p, h = r[l], g = t[f], _ = t[d];
				if (u) {
					let n = e.mul(g, h);
					t[d] = e.add(_, n), t[f] = e.sub(_, n);
				} else a ? (t[d] = e.add(g, _), t[f] = e.mul(e.sub(g, _), h)) : (t[d] = e.add(_, g), t[f] = e.mul(e.sub(_, g), h));
			}
		}
		return !i && s && am(t), t;
	};
}, sm = ip, cm = vp;
function lm(e, t, n = () => {}) {
	if (!Array.isArray(e)) throw TypeError(`"${t}" expected array, got type=${typeof e}`);
	for (let r = 0; r < e.length; r++) n(e[r], `${t}[${r}]`);
	return e;
}
function um(e, t = "object") {
	if (typeof e != "object" || !e || Array.isArray(e)) throw TypeError(t === "object" ? "expected valid options object" : `"${t}" expected object, got type=${typeof e}`);
	return e;
}
function dm(e, t) {
	if (e = ip(e), t = ip(t), e.length !== t.length) return !1;
	let n = 0;
	for (let r = 0; r < e.length; r++) n |= e[r] ^ t[r];
	return n === 0;
}
function fm(e) {
	return Uint8Array.from(ip(e));
}
function pm(e) {
	if (ep(e)) throw TypeError("\"opts\" expected object, got Uint8Array");
	um(e, "opts");
}
function mm(e) {
	pm(e), e.context !== void 0 && ip(e.context, void 0, "opts.context");
}
function hm(e) {
	mm(e), e.extraEntropy !== !1 && e.extraEntropy !== void 0 && ip(e.extraEntropy, void 0, "opts.extraEntropy");
}
function gm(e, ...t) {
	let n = (e) => typeof e == "number" ? e : e.bytesLen, r = t.reduce((e, t) => e + n(t), 0);
	return {
		bytesLen: r,
		encode: (i) => {
			let a = new Uint8Array(r);
			for (let r = 0, o = 0; r < t.length; r++) {
				let s = t[r], c = n(s), l = typeof s == "number" ? i[r] : s.encode(i[r]);
				ip(l, c, e), a.set(l, o), typeof s != "number" && l.fill(0), o += c;
			}
			return a;
		},
		decode: (i) => {
			ip(i, r, e);
			let a = [];
			for (let e of t) {
				let t = n(e), r = i.subarray(0, t);
				a.push(typeof e == "number" ? r : e.decode(r)), i = i.subarray(t);
			}
			return a;
		}
	};
}
function _m(e, t) {
	let n = e, r = t * n.bytesLen;
	return {
		bytesLen: r,
		encode: (e) => {
			let i = lm(e, "u");
			if (i.length !== t) throw RangeError(`vecCoder.encode: wrong length=${i.length}. Expected: ${t}`);
			let a = new Uint8Array(r);
			for (let e = 0, t = 0; e < i.length; e++) {
				let r = n.encode(i[e]);
				a.set(r, t), r.fill(0), t += r.length;
			}
			return a;
		},
		decode: (e) => {
			ip(e, r);
			let t = [];
			for (let r = 0; r < e.length; r += n.bytesLen) t.push(n.decode(e.subarray(r, r + n.bytesLen)));
			return t;
		}
	};
}
function vm(...e) {
	for (let t of e) if (Array.isArray(t)) for (let e of t) e.fill(0);
	else t.fill(0);
}
function ym(e) {
	if (np(e, "bits"), e > 32) throw RangeError("\"bits\" expected <= 32, got " + e);
	return e === 32 ? 4294967295 : ~(-1 << e) >>> 0;
}
var bm = /* @__PURE__ */ Uint8Array.of();
function xm(e, t = bm) {
	if (ip(e, void 0, "msg"), ip(t, void 0, "ctx"), t.length > 255) throw RangeError("context should be 255 bytes or less");
	return hp(new Uint8Array([0, t.length]), t, e);
}
var Sm = /* @__PURE__ */ Uint8Array.from([
	6,
	9,
	96,
	134,
	72,
	1,
	101,
	3,
	4,
	2
]);
function Cm(e, t = 0) {
	if (typeof e != "function" || typeof e.create != "function") throw TypeError("\"hash\" expected hash function, got type=" + typeof e);
	ap(e), np(t, "requiredStrength");
	let n = e.oid;
	if (ip(n, void 0, "hash.oid"), !dm(n.subarray(0, 10), Sm)) throw Error("\"hash.oid\" is invalid: expected NIST hash");
	let r = e.outputLen * 8 / 2;
	if (t > r) throw Error("Pre-hash security strength too low: " + r + ", required: " + t);
}
function wm(e, t, n = bm) {
	if (Cm(e), ip(t, void 0, "msg"), ip(n, void 0, "ctx"), n.length > 255) throw RangeError("context should be 255 bytes or less");
	let r = e(t);
	return hp(new Uint8Array([1, n.length]), n, e.oid, r);
}
//#endregion
//#region node_modules/@noble/post-quantum/_crystals.js
var Tm = (e) => {
	let { newPoly: t, N: n, Q: r, F: i, ROOT_OF_UNITY: a, brvBits: o, isKyber: s } = e, c = (e, t = r) => {
		let n = e % t | 0;
		return (n >= 0 ? n | 0 : t + n | 0) | 0;
	}, l = (e, t = r) => {
		let n = c(e, t) | 0;
		return (n > t >> 1 ? n - t | 0 : n) | 0;
	};
	function u() {
		let e = t(n);
		for (let t = 0; t < n; t++) {
			let n = rm(t, o), i = BigInt(a) ** BigInt(n) % BigInt(r);
			e[t] = Number(i) | 0;
		}
		return e;
	}
	let d = u(), f = (e) => {
		throw Error("not implemented");
	}, p = s ? {
		add: (e, t) => {
			let n = e + t | 0;
			return n >= r ? n - r | 0 : n;
		},
		sub: (e, t) => {
			let n = e - t | 0;
			return n < 0 ? n + r | 0 : n;
		},
		mul: (e, t) => c((e | 0) * (t | 0)) | 0,
		inv: f
	} : {
		add: (e, t) => c((e | 0) + (t | 0)) | 0,
		sub: (e, t) => c((e | 0) - (t | 0)) | 0,
		mul: (e, t) => c((e | 0) * (t | 0)) | 0,
		inv: f
	}, m = {
		N: n,
		roots: d,
		invertButterflies: !0,
		skipStages: +!!s,
		brp: !1
	}, h = om(p, {
		dit: !1,
		...m
	}), g = om(p, {
		dit: !0,
		...m
	}), _ = {
		encode: (e) => h(e),
		decode: (e) => {
			g(e);
			for (let t = 0; t < e.length; t++) e[t] = c(i * e[t]);
			return e;
		}
	};
	return {
		mod: c,
		smod: l,
		nttZetas: d,
		NTT: {
			encode: (e) => _.encode(e),
			decode: (e) => _.decode(e)
		},
		bitsCoder: (e, r) => {
			for (let t = 0, r = 0; t < n; t++) r += e, r > 32 && ym(r), r %= 8;
			let i = ym(e), a = n / 8 * e;
			return {
				bytesLen: a,
				encode: (t) => {
					let n = t, o = new Uint8Array(a);
					for (let t = 0, a = 0, s = 0, c = 0; t < n.length; t++) for (a |= (r.encode(n[t]) & i) << s, s += e; s >= 8; s -= 8, a >>= 8) o[c++] = a & 255;
					return o;
				},
				decode: (a) => {
					let o = t(n);
					for (let t = 0, n = 0, s = 0, c = 0; t < a.length; t++) for (n |= a[t] << s, s += 8; s >= e; s -= e, n >>= e) o[c++] = r.decode(n & i);
					return o;
				}
			};
		}
	};
}, Em = (e) => (t, n) => {
	n ||= e.blockLen;
	let r = new Uint8Array(t.length + 2);
	r.set(t);
	let i = t.length, a = new Uint8Array(n), o = e.create({}), s = 0, c = 0;
	return {
		stats: () => ({
			calls: s,
			xofs: c
		}),
		get: (t, n) => (r[i + 0] = t, r[i + 1] = n, o.destroy(), o = e.create({}).update(r), s++, () => (c++, o.xofInto(a))),
		clean: () => {
			o.destroy(), vm(a, r);
		}
	};
}, Dm = /* @__PURE__ */ Em($p), Om = /* @__PURE__ */ Em(em);
//#endregion
//#region node_modules/@noble/post-quantum/ml-dsa.js
function km(e) {
	pm(e), e.externalMu !== void 0 && Sp(e.externalMu, "opts.externalMu");
}
var Am = 256, jm = 8380417, Mm = 1753, Nm = 8347681, Pm = 13, Fm = 95232, Im = 261888, Lm = /* @__PURE__ */ Object.freeze({
	2: Object.freeze({
		K: 4,
		L: 4,
		D: Pm,
		GAMMA1: 2 ** 17,
		GAMMA2: Fm,
		TAU: 39,
		ETA: 2,
		OMEGA: 80
	}),
	3: Object.freeze({
		K: 6,
		L: 5,
		D: Pm,
		GAMMA1: 2 ** 19,
		GAMMA2: Im,
		TAU: 49,
		ETA: 4,
		OMEGA: 55
	}),
	5: Object.freeze({
		K: 8,
		L: 7,
		D: Pm,
		GAMMA1: 2 ** 19,
		GAMMA2: Im,
		TAU: 60,
		ETA: 2,
		OMEGA: 75
	})
}), Rm = (e) => new Int32Array(e), Q = /* @__PURE__ */ Tm({
	N: Am,
	Q: jm,
	F: Nm,
	ROOT_OF_UNITY: Mm,
	newPoly: Rm,
	isKyber: !1,
	brvBits: 8
}), zm = (e) => e, Bm = (e, t = zm, n = zm) => Q.bitsCoder(e, {
	encode: (e) => t(n(e)),
	decode: (e) => n(t(e))
}), Vm = (e, t) => {
	let n = e, r = t;
	for (let e = 0; e < n.length; e++) n[e] = Q.mod(n[e] + r[e]);
	return n;
}, Hm = (e, t) => {
	let n = e, r = t;
	for (let e = 0; e < n.length; e++) n[e] = Q.mod(n[e] - r[e]);
	return n;
}, Um = (e) => {
	let t = e;
	for (let e = 0; e < Am; e++) t[e] <<= Pm;
	return t;
}, Wm = (e, t) => {
	let n = e;
	for (let e = 0; e < Am; e++) if (Math.abs(Q.smod(n[e])) >= t) return !0;
	return !1;
}, Gm = (e, t) => {
	let n = e, r = t, i = Rm(Am);
	for (let e = 0; e < n.length; e++) i[e] = Q.mod(n[e] * r[e]);
	return i;
};
function Km(e) {
	let t = e, n = Rm(Am);
	for (let e = 0; e < Am;) {
		let r = t();
		if (r.length % 3) throw Error("RejNTTPoly: unaligned block");
		for (let t = 0; e < Am && t <= r.length - 3; t += 3) {
			let i = (r[t + 0] | r[t + 1] << 8 | r[t + 2] << 16) & 8388607;
			i < jm && (n[e++] = i);
		}
	}
	return n;
}
function qm(e) {
	let t = e, { K: n, L: r, GAMMA1: i, GAMMA2: a, TAU: o, ETA: s, OMEGA: c } = t, { CRH_BYTES: l, TR_BYTES: u, C_TILDE_BYTES: d, XOF128: f, XOF256: p, securityLevel: m } = t;
	if (![2, 4].includes(s)) throw Error("Wrong ETA");
	if (![1 << 17, 1 << 19].includes(i)) throw Error("Wrong GAMMA1");
	if (![Fm, Im].includes(a)) throw Error("Wrong GAMMA2");
	let h = o * s, g = (e) => {
		let t = Q.mod(e), n = Q.smod(t, 2 * a) | 0;
		return t - n === 8380416 ? {
			r1: 0,
			r0: n - 1 | 0
		} : {
			r1: Math.floor((t - n) / (2 * a)) | 0,
			r0: n
		};
	}, _ = (e) => g(e).r1, v = (e) => g(e).r0, y = (e, t) => e <= a || e > jm - a || e === jm - a && t === 0 ? 0 : 1, b = Math.floor(8380416 / (2 * a)), x = (e, t) => {
		let { r1: n, r0: r } = g(t);
		return e === 1 ? r > 0 ? Q.mod(n + 1, b) | 0 : Q.mod(n - 1, b) | 0 : n | 0;
	}, S = (e) => {
		let t = Q.mod(e), n = Q.smod(t, 2 ** Pm) | 0;
		return {
			r1: Math.floor((t - n) / 2 ** Pm) | 0,
			r0: n
		};
	}, C = {
		bytesLen: c + n,
		encode: (e) => {
			let t = e;
			if (t === !1) throw Error("hint.encode: hint is false");
			let r = new Uint8Array(c + n);
			for (let e = 0, i = 0; e < n; e++) {
				for (let n = 0; n < Am; n++) t[e][n] !== 0 && (r[i++] = n);
				r[c + e] = i;
			}
			return r;
		},
		decode: (e) => {
			let t = [], r = 0;
			for (let i = 0; i < n; i++) {
				let n = Rm(Am);
				if (e[c + i] < r || e[c + i] > c) return !1;
				for (let t = r; t < e[c + i]; t++) {
					if (t > r && e[t] <= e[t - 1]) return !1;
					n[e[t]] = 1;
				}
				r = e[c + i], t.push(n);
			}
			for (let t = r; t < c; t++) if (e[t] !== 0) return !1;
			return t;
		}
	}, w = Bm(s === 2 ? 3 : 4, (e) => s - e, (e) => {
		if (!(-s <= e && e <= s)) throw Error(`malformed key s1/s3 ${e} outside of ETA range [${-s}, ${s}]`);
		return e;
	}), T = Bm(13, (e) => 4096 - e), E = Bm(10), D = Bm(i === 1 << 17 ? 18 : 20, (e) => Q.smod(i - e)), O = _m(Bm(a === Fm ? 6 : 4), n), k = gm("publicKey", 32, _m(E, n)), A = gm("secretKey", 32, 32, u, _m(w, r), _m(w, n), _m(T, n)), j = gm("signature", d, _m(D, r), C), M = s === 2 ? (e) => e < 15 && 2 - e % 5 : (e) => e < 9 && 4 - e;
	function N(e) {
		let t = e, n = Rm(Am);
		for (let e = 0; e < Am;) {
			let r = t();
			for (let t = 0; e < Am && t < r.length; t += 1) {
				let i = M(r[t] & 15), a = M(r[t] >> 4 & 15);
				i !== !1 && (n[e++] = i), e < Am && a !== !1 && (n[e++] = a);
			}
		}
		return n;
	}
	let P = (e) => {
		let t = Rm(Am), n = em.create({}).update(e), r = new Uint8Array(em.blockLen);
		n.xofInto(r);
		let i = r.slice(0, 8);
		for (let e = Am - o, a = 8, s = 0, c = 0; e < Am; e++) {
			let o = e + 1;
			for (; o > e;) o = r[a++], !(a < em.blockLen) && (n.xofInto(r), a = 0);
			t[e] = t[o], t[o] = 1 - ((i[s] >> c++ & 1) << 1), c >= 8 && (s++, c = 0);
		}
		return t;
	}, F = (e) => {
		let t = e, n = Rm(Am), r = Rm(Am);
		for (let e = 0; e < t.length; e++) {
			let { r0: i, r1: a } = S(t[e]);
			n[e] = i, r[e] = a;
		}
		return {
			r0: n,
			r1: r
		};
	}, I = (e, t) => {
		let n = e, r = t;
		for (let e = 0; e < Am; e++) n[e] = x(r[e], n[e]);
		return n;
	}, L = (e, t) => {
		let n = e, r = t, i = Rm(Am), a = 0;
		for (let e = 0; e < Am; e++) {
			let t = y(n[e], r[e]);
			i[e] = t, a += t;
		}
		return {
			v: i,
			cnt: a
		};
	}, R = gm("seed", 32, 64, 32), z = Object.freeze({
		info: Object.freeze({ type: "internal-ml-dsa" }),
		lengths: Object.freeze({
			secretKey: A.bytesLen,
			publicKey: k.bytesLen,
			seed: 32,
			signature: j.bytesLen,
			signRand: 32
		}),
		keygen: (e) => {
			let t = /* @__PURE__ */ new Uint8Array(34), i = e === void 0;
			i && (e = cm(32)), sm(e, 32, "seed"), t.set(e), i && vm(e), t[32] = n, t[33] = r;
			let [a, o, s] = R.decode(em(t, { dkLen: R.bytesLen })), c = p(o), l = [];
			for (let e = 0; e < r; e++) l.push(N(c.get(e & 255, e >> 8 & 255)));
			let d = [];
			for (let e = r; e < r + n; e++) d.push(N(c.get(e & 255, e >> 8 & 255)));
			let m = l.map((e) => Q.NTT.encode(e.slice())), h = [], g = [], _ = f(a), v = Rm(Am);
			for (let e = 0; e < n; e++) {
				vm(v);
				for (let t = 0; t < r; t++) Vm(v, Gm(Km(_.get(t, e)), m[t]));
				Q.NTT.decode(v);
				let { r0: t, r1: n } = F(Vm(v, d[e]));
				h.push(t), g.push(n);
			}
			let y = k.encode([a, g]), b = em(y, { dkLen: u }), x = A.encode([
				a,
				s,
				b,
				l,
				d,
				h
			]);
			return _.clean(), c.clean(), vm(a, o, s, l, d, m, v, h, g, b, t), {
				publicKey: y,
				secretKey: x
			};
		},
		getPublicKey: (e) => {
			let [t, i, a, o, s, c] = A.decode(e), l = f(t), u = o.map((e) => Q.NTT.encode(e.slice())), d = [], p = Rm(Am);
			for (let e = 0; e < n; e++) {
				p.fill(0);
				for (let t = 0; t < r; t++) Vm(p, Gm(Km(l.get(t, e)), u[t]));
				Q.NTT.decode(p), Vm(p, s[e]);
				let { r1: t } = F(p);
				d.push(t);
			}
			return l.clean(), vm(p, u, c, o, s), k.encode([t, d]);
		},
		sign: (e, t, o = {}) => {
			hm(o), km(o);
			let { extraEntropy: s, externalMu: u = !1 } = o;
			u && sm(e, l, "mu");
			let m = s === !1 || s === void 0, g = s === !1 ? /* @__PURE__ */ new Uint8Array(32) : s === void 0 ? cm(32) : s;
			sm(g, 32, "extraEntropy");
			let [y, b, x, S, C, w] = (() => {
				try {
					return A.decode(t);
				} catch (e) {
					throw m && vm(g), e;
				}
			})(), T = [], E = f(y);
			for (let e = 0; e < n; e++) {
				let t = [];
				for (let n = 0; n < r; n++) t.push(Km(E.get(n, e)));
				T.push(t);
			}
			E.clean();
			for (let e = 0; e < r; e++) Q.NTT.encode(S[e]);
			for (let e = 0; e < n; e++) Q.NTT.encode(C[e]), Q.NTT.encode(w[e]);
			let k = u ? e : em.create({ dkLen: l }).update(x).update(e).digest(), M = em.create({ dkLen: l }).update(b).update(g).update(k).digest();
			m && vm(g), sm(M, l);
			let N = p(M, D.bytesLen);
			main_loop: for (let e = 0;;) {
				let t = [];
				for (let n = 0; n < r; n++, e++) t.push(D.decode(N.get(e & 255, e >> 8)()));
				let o = t.map((e) => Q.NTT.encode(e.slice())), s = [];
				for (let e = 0; e < n; e++) {
					let t = Rm(Am);
					for (let n = 0; n < r; n++) Vm(t, Gm(T[e][n], o[n]));
					Q.NTT.decode(t), s.push(t);
				}
				let l = s.map((e) => e.map(_)), f = em.create({ dkLen: d }).update(k).update(O.encode(l)).digest(), p = Q.NTT.encode(P(f)), m = S.map((e) => Gm(e, p));
				for (let e = 0; e < r; e++) if (Vm(Q.NTT.decode(m[e]), t[e]), Wm(m[e], i - h)) continue main_loop;
				let g = 0, y = [];
				for (let e = 0; e < n; e++) {
					let t = Q.NTT.decode(Gm(C[e], p)), n = Hm(s[e], t).map(v);
					if (Wm(n, a - h)) continue main_loop;
					let r = Q.NTT.decode(Gm(w[e], p));
					if (Wm(r, a)) continue main_loop;
					Vm(n, r);
					let i = L(n, l[e]);
					y.push(i.v), g += i.cnt;
				}
				if (g > c) continue;
				N.clean();
				let b = j.encode([
					f,
					m,
					y
				]);
				return vm(f, m, y, p, l, s, o, t, M, S, C, w, ...T), u || vm(k), b;
			}
			throw Error("Unreachable code path reached, report this error");
		},
		verify: (e, t, a, o = {}) => {
			km(o);
			let { externalMu: s = !1 } = o;
			s && sm(t, l, "mu");
			let [p, m] = k.decode(a), g = em(a, { dkLen: u });
			if (e.length !== j.bytesLen) return !1;
			let [_, v, y] = j.decode(e);
			if (y === !1) return !1;
			for (let e = 0; e < r; e++) if (Wm(v[e], i - h)) return !1;
			let b = s ? t : em.create({ dkLen: l }).update(g).update(t).digest(), x = Q.NTT.encode(P(_)), S = v.map((e) => e.slice());
			for (let e = 0; e < r; e++) Q.NTT.encode(S[e]);
			let C = [], w = f(p);
			for (let e = 0; e < n; e++) {
				let t = Gm(Q.NTT.encode(Um(m[e])), x), n = Rm(Am);
				for (let t = 0; t < r; t++) Vm(n, Gm(Km(w.get(t, e)), S[t]));
				let i = Q.NTT.decode(Hm(n, t));
				C.push(I(i, y[e]));
			}
			w.clean();
			let T = em.create({ dkLen: d }).update(b).update(O.encode(C)).digest();
			for (let e of y) if (!(e.reduce((e, t) => e + t, 0) <= c)) return !1;
			for (let e of v) if (Wm(e, i - h)) return !1;
			return dm(_, T);
		}
	});
	return Object.freeze({
		info: Object.freeze({ type: "ml-dsa" }),
		internal: z,
		securityLevel: m,
		keygen: z.keygen,
		lengths: z.lengths,
		getPublicKey: z.getPublicKey,
		sign: (e, t, n = {}) => {
			hm(n);
			let r = xm(e, n.context), i = z.sign(r, t, n);
			return vm(r), i;
		},
		verify: (e, t, n, r = {}) => (mm(r), sm(e, void 0, "signature"), z.verify(e, xm(t, r.context), n)),
		prehash: (e) => {
			Cm(e, m);
			let t = e;
			return Object.freeze({
				info: Object.freeze({ type: "hashml-dsa" }),
				securityLevel: m,
				lengths: z.lengths,
				keygen: z.keygen,
				getPublicKey: z.getPublicKey,
				sign: (e, n, r = {}) => {
					hm(r);
					let i = wm(t, e, r.context), a = z.sign(i, n, r);
					return vm(i), a;
				},
				verify: (e, n, r, i = {}) => (mm(i), sm(e, void 0, "signature"), z.verify(e, wm(t, n, i.context), r))
			});
		}
	});
}
var Jm = /* @__PURE__ */ qm({
	...Lm[5],
	CRH_BYTES: 64,
	TR_BYTES: 64,
	C_TILDE_BYTES: 64,
	XOF128: Dm,
	XOF256: Om,
	securityLevel: 256
}), Ym = "ML-DSA-87", Xm = "tkmchain:pq-address:v1:";
function Zm(e) {
	return e.Crypto && !e.crypto && (e.crypto = e.Crypto, delete e.Crypto), e.address && (e.address = e.address.toLowerCase(), e.address.startsWith("0x") && (e.address = e.address.slice(2))), e;
}
function Qm(e) {
	if (typeof e != "string") return "";
	let t = e.startsWith("0x") ? e.slice(2) : e;
	return !/^[0-9a-fA-F]+$/.test(t) || t.length !== 5184 ? "" : K(T(W(C([
		H(Xm),
		H(Ym),
		_("0x" + t)
	])), 12));
}
//#endregion
//#region vendor/pq.js
var $m = 6, eh = 32, th = 32;
function nh(e, t) {
	try {
		return _(e);
	} catch {
		throw Error(`${t} is not valid hexadecimal data`);
	}
}
function rh(e) {
	let t = e?.crypto || e?.Crypto;
	if (!t) throw Error("PQ keyfile is missing its crypto section");
	return t;
}
function ih(e, t) {
	if (e.length !== t.length) return !1;
	let n = 0;
	for (let r = 0; r < e.length; r++) n |= e[r] ^ t[r];
	return n === 0;
}
async function ah(e, t) {
	let n = String(e.kdf || "").toLowerCase(), r = e.kdfparams || {}, i = nh("0x" + String(r.salt || "").replace(/^0x/, ""), "KDF salt"), a = H(t), o = Number(r.dklen);
	if (o !== eh) throw Error("PQ keyfile KDF must derive 32 bytes");
	if (n === "scrypt") {
		let e = Number(r.n), t = Number(r.r), n = Number(r.p);
		if (!Number.isSafeInteger(e) || e <= 1 || e & e - 1 || t <= 0 || n <= 0) throw Error("PQ keyfile has invalid scrypt parameters");
		return nh(await fr(a, i, e, t, n, o), "derived key");
	}
	if (n === "pbkdf2") {
		let e = Number(r.c), t = String(r.prf || "").toLowerCase().split("-").pop();
		if (!Number.isSafeInteger(e) || e <= 0 || t !== "sha256" && t !== "sha512") throw Error("PQ keyfile has invalid PBKDF2 parameters");
		return nh(Yn(a, i, e, o, t), "derived key");
	}
	throw Error(`unsupported PQ keyfile KDF: ${n || "missing"}`);
}
function oh(e, t) {
	let n = Jm.keygen(t), r = nh("0x" + String(e.publicKey || "").replace(/^0x/, ""), "PQ public key");
	try {
		if (r.length !== 2592 || !ih(n.publicKey, r)) throw Error("PQ keyfile public key does not match its encrypted seed");
		if (Qm(S(n.publicKey)) !== K("0x" + String(e.address || "").replace(/^0x/, ""))) throw Error("PQ keyfile address does not match its encrypted seed");
		return n;
	} catch (e) {
		throw n.secretKey.fill(0), e;
	}
}
async function sh(e, t) {
	if (Number(e?.version) !== 4 || e?.algorithm !== "ML-DSA-87") throw Error("a version-4 ML-DSA-87 keyfile is required");
	if (typeof t != "string" || t.length === 0) throw Error("PQ keyfile password is required");
	let n = rh(e);
	if (String(n.cipher || "").toLowerCase() !== "aes-128-ctr") throw Error("PQ keyfile cipher must be aes-128-ctr");
	let r = await ah(n, t), i = nh("0x" + String(n.ciphertext || "").replace(/^0x/, ""), "ciphertext"), a = String(n.mac || "").replace(/^0x/, "").toLowerCase();
	if (W(C([r.slice(16, 32), i])).slice(2) !== a) throw r.fill(0), Error("incorrect PQ keyfile password");
	let o = nh("0x" + String(n.cipherparams?.iv || "").replace(/^0x/, ""), "cipher IV"), s = $f(r.slice(0, 16), o).decrypt(i);
	if (r.fill(0), s.length !== th) throw s.fill(0), Error(`PQ keyfile seed has ${s.length} bytes, want ${th}`);
	try {
		return oh(e, s).secretKey.fill(0), s;
	} catch (e) {
		throw s.fill(0), e;
	}
}
function ch(e, t) {
	let n = I(e, t);
	return n === 0n ? "0x" : te(n);
}
function lh(e = []) {
	return e.map((e) => {
		let t = Array.isArray(e) ? e[0] : e.address, n = Array.isArray(e) ? e[1] : e.storageKeys;
		return [K(t), (n || []).map((e) => S(e))];
	});
}
function uh(e, t) {
	let n = Jm.keygen(t), r = S(n.publicKey), i = [
		ch(e.chainId, "chainId"),
		ch(e.nonce, "nonce"),
		ch(e.gasTipCap ?? e.gasPrice, "gasTipCap"),
		ch(e.gasFeeCap ?? e.gasPrice, "gasFeeCap"),
		ch(e.gas, "gas"),
		e.to == null ? "0x" : K(e.to),
		ch(e.value ?? 0, "value"),
		S(e.data ?? e.input ?? "0x"),
		lh(e.accessList),
		S(H(Ym)),
		r
	], a = W(C([te($m, 1), Qe(i)]));
	try {
		let e = Jm.sign(_(a), n.secretKey, { extraEntropy: !1 });
		if (!Jm.verify(e, _(a), n.publicKey)) throw Error("local ML-DSA-87 signature verification failed");
		let t = C([te($m, 1), Qe([...i, S(e)])]);
		return {
			rawTransaction: t,
			transactionHash: W(t),
			signingHash: a,
			publicKey: r
		};
	} finally {
		n.secretKey.fill(0);
	}
}
//#endregion
//#region node_modules/@noble/curves/esm/abstract/montgomery.js
var dh = BigInt(0), fh = BigInt(1);
function ph(e) {
	return Gr(e, { a: "bigint" }, {
		montgomeryBits: "isSafeInteger",
		nByteLength: "isSafeInteger",
		adjustScalarBytes: "function",
		domain: "function",
		powPminus2: "function",
		Gu: "bigint"
	}), Object.freeze({ ...e });
}
function mh(e) {
	let t = ph(e), { P: n } = t, r = (e) => ei(e, n), i = t.montgomeryBits, a = Math.ceil(i / 8), o = t.nByteLength, s = t.adjustScalarBytes || ((e) => e), c = t.powPminus2 || ((e) => ti(e, n - BigInt(2), n));
	function l(e, t, n) {
		let i = r(e * (t - n));
		return t = r(t - i), n = r(n + i), [t, n];
	}
	function u(e) {
		if (typeof e == "bigint" && dh <= e && e < n) return e;
		throw Error("Expected valid scalar 0 < scalar < CURVE.P");
	}
	let d = (t.a - BigInt(2)) / BigInt(4);
	function f(e, t) {
		let n = u(e), a = u(t), o = n, s = fh, f = dh, p = n, m = fh, h = dh, g;
		for (let e = BigInt(i - 1); e >= dh; e--) {
			let t = a >> e & fh;
			h ^= t, g = l(h, s, p), s = g[0], p = g[1], g = l(h, f, m), f = g[0], m = g[1], h = t;
			let n = s + f, i = r(n * n), c = s - f, u = r(c * c), _ = i - u, v = p + m, y = p - m, b = r(y * n), x = r(v * c), S = b + x, C = b - x;
			p = r(S * S), m = r(o * r(C * C)), s = r(i * u), f = r(_ * (i + r(d * _)));
		}
		g = l(h, s, p), s = g[0], p = g[1], g = l(h, f, m), f = g[0], m = g[1];
		let _ = c(f);
		return r(s * _);
	}
	function p(e) {
		return jr(r(e), a);
	}
	function m(e) {
		let t = Nr("u coordinate", e, a);
		return o === a && (t[o - 1] &= 127), kr(t);
	}
	function h(e) {
		let t = Nr("scalar", e);
		if (t.length !== a && t.length !== o) throw Error(`Expected ${a} or ${o} bytes, got ${t.length}`);
		return kr(s(t));
	}
	function g(e, t) {
		let n = f(m(t), h(e));
		if (n === dh) throw Error("Invalid private or public key received");
		return p(n);
	}
	let _ = p(t.Gu);
	function v(e) {
		return g(e, _);
	}
	return {
		scalarMult: g,
		scalarMultBase: v,
		getSharedSecret: (e, t) => g(e, t),
		getPublicKey: (e) => v(e),
		utils: { randomPrivateKey: () => t.randomBytes(t.nByteLength) },
		GuBytes: _
	};
}
//#endregion
//#region node_modules/@noble/curves/esm/ed25519.js
var hh = BigInt("57896044618658097711785492504343953926634992332820282019728792003956564819949"), gh = BigInt("19681161376707505956807079304988542015446066515923890162744021073123829784752"), _h = BigInt(1), vh = BigInt(2), yh = BigInt(5), bh = BigInt(10), xh = BigInt(20), Sh = BigInt(40), Ch = BigInt(80);
function wh(e) {
	let t = hh, n = e * e % t * e % t, r = ni(ni(n, vh, t) * n % t, _h, t) * e % t, i = ni(r, yh, t) * r % t, a = ni(i, bh, t) * i % t, o = ni(a, xh, t) * a % t, s = ni(o, Sh, t) * o % t;
	return {
		pow_p_5_8: ni(ni(ni(ni(s, Ch, t) * s % t, Ch, t) * s % t, bh, t) * i % t, vh, t) * e % t,
		b2: n
	};
}
function Th(e) {
	return e[0] &= 248, e[31] &= 127, e[31] |= 64, e;
}
function Eh(e, t) {
	let n = hh, r = ei(t * t * t, n), i = wh(e * ei(r * r * t, n)).pow_p_5_8, a = ei(e * r * i, n), o = ei(t * a * a, n), s = a, c = ei(a * gh, n), l = o === e, u = o === ei(-e, n), d = o === ei(-e * gh, n);
	return l && (a = s), (u || d) && (a = c), oi(a, n) && (a = ei(-a, n)), {
		isValid: l || u,
		value: a
	};
}
var Dh = fi(hh, void 0, !0), Oh = {
	a: BigInt(-1),
	d: BigInt("37095705934669439343138083508754565189542113879843219016388785533085940283555"),
	Fp: Dh,
	n: BigInt("7237005577332262213973186563042994240857116359379907606001950938285454250989"),
	h: BigInt(8),
	Gx: BigInt("15112221349535400772501151409588531511454012693041857206046113283949847762202"),
	Gy: BigInt("46316835694926478169428394003475163141307993866256225615783033603165251855960"),
	hash: bn,
	randomBytes: Ft,
	adjustScalarBytes: Th,
	uvRatio: Eh
};
({ ...Oh }), { ...Oh };
var kh = /* @__PURE__ */ mh({
	P: hh,
	a: BigInt(486662),
	montgomeryBits: 255,
	nByteLength: 32,
	Gu: BigInt(9),
	powPminus2: (e) => {
		let t = hh, { pow_p_5_8: n, b2: r } = wh(e);
		return ei(ni(n, BigInt(3), t) * r, t);
	},
	adjustScalarBytes: Th,
	randomBytes: Ft
}), Ah = (Dh.ORDER + BigInt(3)) / BigInt(8);
Dh.pow(vh, Ah), Dh.sqrt(Dh.neg(Dh.ONE)), (Dh.ORDER - BigInt(5)) / BigInt(8), pi(Dh, Dh.neg(BigInt(486664)));
//#endregion
//#region node_modules/@noble/ciphers/_arx.js
var jh = (e) => Uint8Array.from(e.split(""), (e) => e.charCodeAt(0)), Mh = /* @__PURE__ */ Tf(vf(jh("expand 16-byte k"))), Nh = /* @__PURE__ */ Tf(vf(jh("expand 32-byte k")));
function $(e, t) {
	return e << t | e >>> 32 - t;
}
var Ph = 64, Fh = 16, Ih = 2 ** 32 - 1, Lh = /* @__PURE__ */ Uint32Array.of();
function Rh(e, t, n, r, i, a, o, s) {
	let c = i.length, l = new Uint8Array(Ph), u = vf(l), d = xf && Pf(i) && Pf(a), f = d ? vf(i) : Lh, p = d ? vf(a) : Lh;
	if (!xf) {
		for (let d = 0; d < c; o++) {
			if (e(t, n, r, u, o, s), Tf(u), o >= Ih) throw Error("arx: counter overflow");
			let f = Math.min(Ph, c - d);
			for (let e = 0, t; e < f; e++) t = d + e, a[t] = i[t] ^ l[e];
			d += f;
		}
		return;
	}
	for (let m = 0; m < c; o++) {
		if (e(t, n, r, u, o, s), o >= Ih) throw Error("arx: counter overflow");
		let h = Math.min(Ph, c - m);
		if (d && h === Ph) {
			let e = m / 4;
			if (m % 4 != 0) throw Error("arx: invalid block position");
			for (let t = 0, n; t < Fh; t++) n = e + t, p[n] = f[n] ^ u[t];
			m += Ph;
			continue;
		}
		for (let e = 0, t; e < h; e++) t = m + e, a[t] = i[t] ^ l[e];
		m += h;
	}
}
function zh(e, t) {
	let { allowShortKeys: n, extendNonceFn: r, counterLength: i, counterRight: a, rounds: o } = Of({
		allowShortKeys: !1,
		counterLength: 8,
		counterRight: !1,
		rounds: 20
	}, t);
	if (typeof e != "function") throw Error("core must be a function");
	return ff(i), ff(o), df(a), df(n), (t, s, c, l, u = 0) => {
		pf(t, void 0, "key"), pf(s, void 0, "nonce"), pf(c, void 0, "data");
		let d = c.length;
		if (l = Mf(d, l, !1), ff(u), u < 0 || u >= Ih) throw Error("arx: counter overflow");
		let f = [], p = t.length, m, h;
		if (p === 32) f.push(m = Ff(t)), h = Nh;
		else if (p === 16 && n) m = /* @__PURE__ */ new Uint8Array(32), m.set(t), m.set(t, 16), h = Mh, f.push(m);
		else throw pf(t, 32, "arx key"), Error("invalid key size");
		(!xf || !Pf(s)) && f.push(s = Ff(s));
		let g = vf(m);
		if (r) {
			if (s.length !== 24) throw Error("arx: extended nonce must be 24 bytes");
			let e = s.subarray(0, 16);
			if (xf) r(h, g, vf(e), g);
			else {
				let t = Tf(Uint32Array.from(h));
				r(t, g, vf(e), g), yf(t), Tf(g);
			}
			s = s.subarray(16);
		} else xf || Tf(g);
		let _ = 16 - i;
		if (_ !== s.length) throw Error(`arx: nonce must be ${_} or 16 bytes`);
		if (_ !== 12) {
			let e = /* @__PURE__ */ new Uint8Array(12);
			e.set(s, a ? 0 : 12 - s.length), s = e, f.push(s);
		}
		let v = Tf(vf(s));
		try {
			return Rh(e, h, g, v, c, l, u, o), l;
		} finally {
			yf(...f);
		}
	};
}
//#endregion
//#region node_modules/@noble/ciphers/_poly1305.js
function Bh(e, t) {
	return e[t++] & 255 | (e[t++] & 255) << 8;
}
var Vh = class {
	blockLen = 16;
	outputLen = 16;
	buffer = /* @__PURE__ */ new Uint8Array(16);
	r = /* @__PURE__ */ new Uint16Array(10);
	h = /* @__PURE__ */ new Uint16Array(10);
	pad = /* @__PURE__ */ new Uint16Array(8);
	pos = 0;
	finished = !1;
	destroyed = !1;
	constructor(e) {
		e = Ff(pf(e, 32, "key"));
		let t = Bh(e, 0), n = Bh(e, 2), r = Bh(e, 4), i = Bh(e, 6), a = Bh(e, 8), o = Bh(e, 10), s = Bh(e, 12), c = Bh(e, 14);
		this.r[0] = t & 8191, this.r[1] = (t >>> 13 | n << 3) & 8191, this.r[2] = (n >>> 10 | r << 6) & 7939, this.r[3] = (r >>> 7 | i << 9) & 8191, this.r[4] = (i >>> 4 | a << 12) & 255, this.r[5] = a >>> 1 & 8190, this.r[6] = (a >>> 14 | o << 2) & 8191, this.r[7] = (o >>> 11 | s << 5) & 8065, this.r[8] = (s >>> 8 | c << 8) & 8191, this.r[9] = c >>> 5 & 127;
		for (let t = 0; t < 8; t++) this.pad[t] = Bh(e, 16 + 2 * t);
	}
	process(e, t, n = !1) {
		let r = n ? 0 : 2048, { h: i, r: a } = this, o = a[0], s = a[1], c = a[2], l = a[3], u = a[4], d = a[5], f = a[6], p = a[7], m = a[8], h = a[9], g = Bh(e, t + 0), _ = Bh(e, t + 2), v = Bh(e, t + 4), y = Bh(e, t + 6), b = Bh(e, t + 8), x = Bh(e, t + 10), S = Bh(e, t + 12), C = Bh(e, t + 14), w = i[0] + (g & 8191), T = i[1] + ((g >>> 13 | _ << 3) & 8191), E = i[2] + ((_ >>> 10 | v << 6) & 8191), D = i[3] + ((v >>> 7 | y << 9) & 8191), O = i[4] + ((y >>> 4 | b << 12) & 8191), k = i[5] + (b >>> 1 & 8191), A = i[6] + ((b >>> 14 | x << 2) & 8191), j = i[7] + ((x >>> 11 | S << 5) & 8191), M = i[8] + ((S >>> 8 | C << 8) & 8191), N = i[9] + (C >>> 5 | r), P = 0, F = P + w * o + 5 * h * T + 5 * m * E + 5 * p * D + 5 * f * O;
		P = F >>> 13, F &= 8191, F += 5 * d * k + 5 * u * A + 5 * l * j + 5 * c * M + 5 * s * N, P += F >>> 13, F &= 8191;
		let I = P + w * s + T * o + 5 * h * E + 5 * m * D + 5 * p * O;
		P = I >>> 13, I &= 8191, I += 5 * f * k + 5 * d * A + 5 * u * j + 5 * l * M + 5 * c * N, P += I >>> 13, I &= 8191;
		let L = P + w * c + T * s + E * o + 5 * h * D + 5 * m * O;
		P = L >>> 13, L &= 8191, L += 5 * p * k + 5 * f * A + 5 * d * j + 5 * u * M + 5 * l * N, P += L >>> 13, L &= 8191;
		let R = P + w * l + T * c + E * s + D * o + 5 * h * O;
		P = R >>> 13, R &= 8191, R += 5 * m * k + 5 * p * A + 5 * f * j + 5 * d * M + 5 * u * N, P += R >>> 13, R &= 8191;
		let z = P + w * u + T * l + E * c + D * s + O * o;
		P = z >>> 13, z &= 8191, z += 5 * h * k + 5 * m * A + 5 * p * j + 5 * f * M + 5 * d * N, P += z >>> 13, z &= 8191;
		let ee = P + w * d + T * u + E * l + D * c + O * s;
		P = ee >>> 13, ee &= 8191, ee += k * o + 5 * h * A + 5 * m * j + 5 * p * M + 5 * f * N, P += ee >>> 13, ee &= 8191;
		let te = P + w * f + T * d + E * u + D * l + O * c;
		P = te >>> 13, te &= 8191, te += k * s + A * o + 5 * h * j + 5 * m * M + 5 * p * N, P += te >>> 13, te &= 8191;
		let B = P + w * p + T * f + E * d + D * u + O * l;
		P = B >>> 13, B &= 8191, B += k * c + A * s + j * o + 5 * h * M + 5 * m * N, P += B >>> 13, B &= 8191;
		let V = P + w * m + T * p + E * f + D * d + O * u;
		P = V >>> 13, V &= 8191, V += k * l + A * c + j * s + M * o + 5 * h * N, P += V >>> 13, V &= 8191;
		let ne = P + w * h + T * m + E * p + D * f + O * d;
		P = ne >>> 13, ne &= 8191, ne += k * u + A * l + j * c + M * s + N * o, P += ne >>> 13, ne &= 8191, P = (P << 2) + P | 0, P = P + F | 0, F = P & 8191, P >>>= 13, I += P, i[0] = F, i[1] = I, i[2] = L, i[3] = R, i[4] = z, i[5] = ee, i[6] = te, i[7] = B, i[8] = V, i[9] = ne;
	}
	finalize() {
		let { h: e, pad: t } = this, n = /* @__PURE__ */ new Uint16Array(10), r = e[1] >>> 13;
		e[1] &= 8191;
		for (let t = 2; t < 10; t++) e[t] += r, r = e[t] >>> 13, e[t] &= 8191;
		e[0] += r * 5, r = e[0] >>> 13, e[0] &= 8191, e[1] += r, r = e[1] >>> 13, e[1] &= 8191, e[2] += r, n[0] = e[0] + 5, r = n[0] >>> 13, n[0] &= 8191;
		for (let t = 1; t < 10; t++) n[t] = e[t] + r, r = n[t] >>> 13, n[t] &= 8191;
		n[9] -= 8192;
		let i = (r ^ 1) - 1;
		for (let e = 0; e < 10; e++) n[e] &= i;
		i = ~i;
		for (let t = 0; t < 10; t++) e[t] = e[t] & i | n[t];
		e[0] = (e[0] | e[1] << 13) & 65535, e[1] = (e[1] >>> 3 | e[2] << 10) & 65535, e[2] = (e[2] >>> 6 | e[3] << 7) & 65535, e[3] = (e[3] >>> 9 | e[4] << 4) & 65535, e[4] = (e[4] >>> 12 | e[5] << 1 | e[6] << 14) & 65535, e[5] = (e[6] >>> 2 | e[7] << 11) & 65535, e[6] = (e[7] >>> 5 | e[8] << 8) & 65535, e[7] = (e[8] >>> 8 | e[9] << 5) & 65535;
		let a = e[0] + t[0];
		e[0] = a & 65535;
		for (let n = 1; n < 8; n++) a = (e[n] + t[n] | 0) + (a >>> 16) | 0, e[n] = a & 65535;
		yf(n);
	}
	update(e) {
		hf(this), pf(e), e = Ff(e);
		let { buffer: t, blockLen: n } = this, r = e.length;
		for (let i = 0; i < r;) {
			let a = Math.min(n - this.pos, r - i);
			if (a === n) {
				for (; n <= r - i; i += n) this.process(e, i);
				continue;
			}
			t.set(e.subarray(i, i + a), this.pos), this.pos += a, i += a, this.pos === n && (this.process(t, 0, !1), this.pos = 0);
		}
		return this;
	}
	destroy() {
		this.destroyed = !0, yf(this.h, this.r, this.buffer, this.pad);
	}
	digestInto(e) {
		hf(this), gf(e, this), this.finished = !0;
		let { buffer: t, h: n } = this, { pos: r } = this;
		if (r) {
			for (t[r++] = 1; r < 16; r++) t[r] = 0;
			this.process(t, 0, !0);
		}
		this.finalize();
		let i = 0;
		for (let t = 0; t < 8; t++) e[i++] = n[t] >>> 0, e[i++] = n[t] >>> 8;
	}
	digest() {
		let { buffer: e, outputLen: t } = this;
		this.digestInto(e);
		let n = e.slice(0, t);
		return this.destroy(), n;
	}
}, Hh = /* @__PURE__ */ Af(32, (e) => new Vh(e));
//#endregion
//#region node_modules/@noble/ciphers/chacha.js
function Uh(e, t, n, r, i, a = 20) {
	let o = e[0], s = e[1], c = e[2], l = e[3], u = t[0], d = t[1], f = t[2], p = t[3], m = t[4], h = t[5], g = t[6], _ = t[7], v = i, y = n[0], b = n[1], x = n[2], S = o, C = s, w = c, T = l, E = u, D = d, O = f, k = p, A = m, j = h, M = g, N = _, P = v, F = y, I = b, L = x;
	for (let e = 0; e < a; e += 2) S = S + E | 0, P = $(P ^ S, 16), A = A + P | 0, E = $(E ^ A, 12), S = S + E | 0, P = $(P ^ S, 8), A = A + P | 0, E = $(E ^ A, 7), C = C + D | 0, F = $(F ^ C, 16), j = j + F | 0, D = $(D ^ j, 12), C = C + D | 0, F = $(F ^ C, 8), j = j + F | 0, D = $(D ^ j, 7), w = w + O | 0, I = $(I ^ w, 16), M = M + I | 0, O = $(O ^ M, 12), w = w + O | 0, I = $(I ^ w, 8), M = M + I | 0, O = $(O ^ M, 7), T = T + k | 0, L = $(L ^ T, 16), N = N + L | 0, k = $(k ^ N, 12), T = T + k | 0, L = $(L ^ T, 8), N = N + L | 0, k = $(k ^ N, 7), S = S + D | 0, L = $(L ^ S, 16), M = M + L | 0, D = $(D ^ M, 12), S = S + D | 0, L = $(L ^ S, 8), M = M + L | 0, D = $(D ^ M, 7), C = C + O | 0, P = $(P ^ C, 16), N = N + P | 0, O = $(O ^ N, 12), C = C + O | 0, P = $(P ^ C, 8), N = N + P | 0, O = $(O ^ N, 7), w = w + k | 0, F = $(F ^ w, 16), A = A + F | 0, k = $(k ^ A, 12), w = w + k | 0, F = $(F ^ w, 8), A = A + F | 0, k = $(k ^ A, 7), T = T + E | 0, I = $(I ^ T, 16), j = j + I | 0, E = $(E ^ j, 12), T = T + E | 0, I = $(I ^ T, 8), j = j + I | 0, E = $(E ^ j, 7);
	let R = 0;
	r[R++] = o + S | 0, r[R++] = s + C | 0, r[R++] = c + w | 0, r[R++] = l + T | 0, r[R++] = u + E | 0, r[R++] = d + D | 0, r[R++] = f + O | 0, r[R++] = p + k | 0, r[R++] = m + A | 0, r[R++] = h + j | 0, r[R++] = g + M | 0, r[R++] = _ + N | 0, r[R++] = v + P | 0, r[R++] = y + F | 0, r[R++] = b + I | 0, r[R++] = x + L | 0;
}
function Wh(e, t, n, r) {
	let i = xf ? e : Tf(e.slice(0, 4)), a = xf ? t : Tf(t.slice(0, 8)), o = xf ? n : Tf(n.slice(0, 4)), s = /* @__PURE__ */ new Uint32Array(16);
	Uh(i, a, o.subarray(1), s, o[0]);
	let c = 0;
	r[c++] = s[0] - i[0] | 0, r[c++] = s[1] - i[1] | 0, r[c++] = s[2] - i[2] | 0, r[c++] = s[3] - i[3] | 0, r[c++] = s[12] - o[0] | 0, r[c++] = s[13] - o[1] | 0, r[c++] = s[14] - o[2] | 0, r[c++] = s[15] - o[3] | 0, Tf(r), xf || yf(i, a, o), yf(s);
}
var Gh = /* @__PURE__ */ zh(Uh, {
	counterRight: !1,
	counterLength: 8,
	extendNonceFn: Wh,
	allowShortKeys: !1
}), Kh = /* @__PURE__ */ new Uint8Array(16), qh = (e, t) => {
	e.update(t);
	let n = t.length % 16;
	n && e.update(Kh.subarray(n));
}, Jh = /* @__PURE__ */ new Uint8Array(32);
function Yh(e, t, n, r, i) {
	i !== void 0 && pf(i, void 0, "AAD");
	let a = e(t, n, Jh), o = Nf(r.length, i ? i.length : 0, !0), s = Hh.create(a);
	i && qh(s, i), qh(s, r), s.update(o);
	let c = s.digest();
	return yf(a, o), c;
}
var Xh = /* @__PURE__ */ jf({
	blockSize: 64,
	nonceLength: 24,
	tagLength: 16,
	withAAD: !0
}, /* @__PURE__ */ ((e) => (t, n, r) => ({
	encrypt(i, a) {
		let o = i.length;
		a = Mf(o + 16, a, !1), a.set(i);
		let s = a.subarray(0, -16);
		e(t, n, s, s, 1);
		let c = Yh(e, t, n, s, r);
		return a.set(c, o), yf(c), a;
	},
	decrypt(i, a) {
		a = Mf(i.length - 16, a, !1);
		let o = i.subarray(0, -16), s = i.subarray(-16), c = Yh(e, t, n, o, r);
		if (!kf(s, c)) throw yf(c), Error("invalid tag");
		return a.set(i.subarray(0, -16)), e(t, n, a, a, 1), yf(c), a;
	}
}))(Gh));
//#endregion
//#region node_modules/@noble/hashes/esm/hkdf.js
function Zh(e, t, n) {
	return vt(e), n === void 0 && (n = new Uint8Array(e.outputLen)), Lt(e, kt(n), kt(t));
}
var Qh = /* @__PURE__ */ new Uint8Array([0]), $h = /* @__PURE__ */ new Uint8Array();
function eg(e, t, n, r = 32) {
	if (vt(e), gt(r), r > 255 * e.outputLen) throw Error("Length should be <= 255*HashLen");
	let i = Math.ceil(r / e.outputLen);
	n === void 0 && (n = $h);
	let a = new Uint8Array(i * e.outputLen), o = Lt.create(e, t), s = o._cloneInto(), c = new Uint8Array(o.outputLen);
	for (let t = 0; t < i; t++) Qh[0] = t + 1, s.update(t === 0 ? $h : c).update(n).update(Qh).digestInto(c), a.set(c, e.outputLen * t), o._cloneInto(s);
	return o.destroy(), s.destroy(), c.fill(0), Qh.fill(0), a.slice(0, r);
}
var tg = (e, t, n, r, i) => eg(e, Zh(e, t, n), r, i), ng = K("0x00000000000000000000000000000000000000f7"), rg = 2001n, ig = 21888242871839275222246405745257275088548364400416034343698204186575808495617n, ag = H("TKMSHIELD1"), og = "TKM_SHIELDED_NOTE_X25519_XCHACHA20POLY1305_V1", sg = "TKM_SHIELDED_VIEW_SALT_V1", cg = "TKM_SHIELDED_VIEW_X25519_V1", lg = "TKM_SHIELDED_STORE_SALT_V1", ug = "TKM_SHIELDED_STORE_AES256_V1";
function dg(e) {
	return _(e);
}
var fg = null;
function pg(e) {
	let t = BigInt(e) % ig;
	return t < 0n ? t + ig : t;
}
function mg(e) {
	let t = e * e % ig;
	return t * t % ig * e % ig;
}
function hg() {
	if (fg) return fg;
	let e = [], t = W(H("seed"));
	for (let n = 0; n < 110; n++) t = W(t), e.push(pg(t));
	return fg = e, e;
}
function gg(e) {
	let t = 0n;
	for (let n of e) {
		let e = pg(n), r = e;
		for (let e of hg()) r = mg(pg(r + t + e));
		r = pg(r + t), t = pg(r + t + e);
	}
	return t;
}
function _g(e) {
	return te(pg(e), 32).toLowerCase();
}
function vg(e, t, n) {
	if (Number(t?.shieldedVersion) !== 2) throw Error("proof builder did not return a Shielded V2 response");
	let r = dg(e?.data || "0x");
	if (r.length <= ag.length || !ag.every((e, t) => r[t] === e)) throw Error("proof builder returned an invalid shielded envelope");
	let i = Je(r.slice(ag.length));
	if (!Array.isArray(i) || BigInt(i[0]) !== 2n || !Array.isArray(i[2]) || i[2].length !== 4) throw Error("proof builder returned an invalid Shielded V2 envelope");
	let a = Array.isArray(t.outputOpenings) ? t.outputOpenings : [];
	if (a.length !== n.length) throw Error("proof builder returned incomplete V2 output openings");
	let o = /* @__PURE__ */ new Set();
	for (let e of n) {
		let t = a.find((t) => Number(t.index) === Number(e.index));
		if (!t || o.has(Number(t.index))) throw Error("proof builder returned an invalid V2 output index");
		o.add(Number(t.index));
		let n = Number(t.index);
		if (!Number.isInteger(n) || n < 0 || n >= 4) throw Error("proof builder returned an invalid V2 output index");
		let r = K(t.recipient);
		if (r !== K(e.recipient) || BigInt(t.valueWei) !== BigInt(e.valueWei) || BigInt(t.assetId) !== BigInt(e.assetId)) throw Error("proof builder changed the requested V2 recipient, value, or asset");
		let s = _g(gg([
			rg,
			BigInt(r),
			BigInt(t.assetId),
			BigInt(t.valueWei),
			BigInt(t.randomness)
		])), c = S(i[2][n][0]).toLowerCase();
		if (s !== String(t.commitment).toLowerCase() || s !== c) throw Error("proof builder returned a V2 output commitment that does not match its opening");
	}
	return e;
}
function yg(e, t, { recipient: n, valueWei: r, expectedOutputs: i = [] }) {
	vg(e, t, i);
	let a = Je(dg(e?.data || "0x").slice(ag.length));
	if (!Array.isArray(a) || a.length < 7) throw Error("proof builder omitted the shielded withdrawal fields");
	let o;
	try {
		o = K(S(a[5]));
	} catch {
		throw Error("proof builder returned an invalid withdrawal recipient");
	}
	if (o !== K(n) || BigInt(a[6]) !== BigInt(r)) throw Error("proof builder changed the withdrawal recipient or value");
	return e;
}
function bg(e, t) {
	let n = dg(e?.data || "0x");
	if (n.length <= ag.length || !ag.every((e, t) => n[t] === e)) throw Error("proof builder returned an invalid shielded envelope");
	let r = Je(n.slice(ag.length)), i = Array.isArray(r) && r.length >= 8 && r[7] !== "0x" ? BigInt(r[7]) : 0n, a = BigInt(t?.gasSponsorWei || 0), o = BigInt(e.gas) * BigInt(e.gasFeeCap);
	if (i !== a || i < 0n || i > o) throw Error("proof builder returned an invalid shielded gas sponsorship");
	return i;
}
function xg(e, t) {
	let n = S(t).toLowerCase(), r = dg(e?.data || "0x");
	if (r.length <= ag.length || !ag.every((e, t) => r[t] === e)) throw Error("proof builder returned an invalid shielded envelope");
	let i = Je(r.slice(ag.length)), a = Array.isArray(i) ? i[1] : null, o = Array.isArray(a) && a.length > 0 ? a[0] : null;
	if (!Array.isArray(o) || o.length < 4 || S(o[3]).toLowerCase() !== n) throw Error("proof builder changed or omitted the domain/mail application data");
	return e;
}
function Sg(e) {
	let t = Array.from(e, (e) => String.fromCharCode(e)).join("");
	return btoa(t).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}
function Cg(e) {
	let t = e.replaceAll("-", "+").replaceAll("_", "/"), n = t + "=".repeat((4 - t.length % 4) % 4);
	return Uint8Array.from(atob(n), (e) => e.charCodeAt(0));
}
function wg(e, t, n = 8979) {
	let r = K(t), i = tg(Yt, e, H(sg), H(cg), 32), a = kh.getPublicKey(i), o = tg(Yt, e, H(lg), H(ug), 32), s = Eg({
		chainId: n,
		address: r,
		viewPublicKey: a,
		version: 1
	}), c = Eg({
		chainId: n,
		address: r,
		viewPublicKey: a,
		version: 2
	});
	return {
		address: r,
		chainId: n,
		viewPrivateKey: i,
		viewPublicKey: a,
		storageKey: o,
		paymentCode: c,
		paymentCodeV1: s,
		paymentCodeV2: c
	};
}
function Tg(e) {
	e && (e.viewPrivateKey?.fill(0), e.storageKey?.fill(0));
}
function Eg({ chainId: e, address: t, viewPublicKey: n, version: r = 2 }) {
	if (r !== 1 && r !== 2) throw Error("unsupported shielded payment-code version");
	let i = {
		v: r,
		c: Number(e),
		a: K(t),
		k: S(n).slice(2)
	};
	return `tkmshield${r}.` + Sg(H(JSON.stringify(i)));
}
function Dg(e, t = 8979) {
	let n = typeof e == "string" ? e.match(/^tkmshield([12])\./) : null;
	if (!n) throw Error("recipient must be a tkmshield1 or tkmshield2 payment code");
	let r = Number(n[1]), i;
	try {
		i = JSON.parse(me(Cg(e.slice(`tkmshield${r}.`.length))));
	} catch {
		throw Error("invalid shielded payment code");
	}
	if (i.v !== r || Number(i.c) !== Number(t)) throw Error(`shielded payment code is not for chain ${t}`);
	let a = K(i.a), o = dg("0x" + String(i.k || "").replace(/^0x/, ""));
	if (o.length !== 32) throw Error("shielded payment code has an invalid viewing public key");
	return {
		version: r,
		chainId: Number(i.c),
		address: a,
		viewPublicKey: o
	};
}
function Og(e, t, n) {
	return tg(Yt, kh.getSharedSecret(e, t), dg(n), H(og), 33);
}
function kg(e, t) {
	let n = dg(e.ephemeralPubKey), r = dg(e.encryptedPayload), i = dg(e.nonce), a = S(e.commitment);
	if (n.length !== 32 || i.length !== 24 || dg(e.viewTag).length !== 1) return null;
	let o;
	try {
		if (o = Og(t.viewPrivateKey, n, a), o[32] !== dg(e.viewTag)[0] || W(r).toLowerCase() !== String(e.payloadHash).toLowerCase()) return null;
		let s = dg(C([H(og), dg(a)])), c = Xh(o.slice(0, 32), i, s).decrypt(r), l = JSON.parse(me(c));
		if (l.format !== "TKM_SHIELDED_NOTE_PAYLOAD_V3" && l.format !== "TKM_SHIELDED_NOTE_PAYLOAD_V4") return null;
		let u = Number(l.version || 1);
		return u !== 1 && u !== 2 || String(l.commitment).toLowerCase() !== a.toLowerCase() || K(l.recipient) !== t.address || BigInt(l.noteValueWei) <= 0n ? null : {
			...l,
			version: u
		};
	} catch {
		return null;
	} finally {
		o?.fill(0);
	}
}
var Ag = class {
	constructor(e, t, n = globalThis.fetch) {
		this.url = String(e || "").replace(/\/$/, ""), this.bearerToken = String(t || "").trim(), this.requestFetch = n;
	}
	async request(e, t = null) {
		if (!this.url) throw Error("configure the local proof-builder URL");
		let n = await this.requestFetch(this.url + e, {
			method: t == null ? "GET" : "POST",
			headers: {
				...this.bearerToken ? { authorization: `Bearer ${this.bearerToken}` } : {},
				...t == null ? {} : { "content-type": "application/json" }
			},
			...t == null ? {} : { body: JSON.stringify(t) }
		}), r = await n.json().catch(() => ({}));
		if (!n.ok || r.error) throw Error(r.error || `proof builder returned HTTP ${n.status}`);
		return r;
	}
	health() {
		return this.request("/healthz");
	}
	buildDeposit(e) {
		return this.request("/build-deposit", e);
	}
	buildTransfer(e) {
		return this.request("/build-transfer", e);
	}
	buildWithdrawal(e) {
		return this.request("/build-withdrawal", e);
	}
};
function jg(e, { chainId: t = 8979, value: n = null } = {}) {
	if (!e || Number(BigInt(e.chainId)) !== Number(t)) throw Error("proof builder returned the wrong chain ID");
	if (K(e.to) !== ng) throw Error("proof builder returned the wrong shielded pool address");
	if (!String(e.data || "").startsWith(S(H("TKMSHIELD1")))) throw Error("proof builder did not return a TKMSHIELD1 envelope");
	if (n != null && BigInt(e.value) !== BigInt(n)) throw Error("proof builder returned an unexpected public value");
	return e;
}
function Mg(e, t) {
	let n = e.findIndex((e) => e.commitment?.toLowerCase() === t.commitment.toLowerCase());
	n >= 0 ? e[n] = {
		...e[n],
		...t
	} : e.push(t);
}
async function Ng(e, t, n) {
	if (n <= 0n) return 0;
	let r = await e.getBlock(t, !1);
	if (!r || BigInt(r.timestamp) < n) return t + 1;
	let i = 0, a = t;
	for (; i < a;) {
		let t = Math.floor((i + a) / 2), r = await e.getBlock(t, !1);
		if (!r) return 0;
		BigInt(r.timestamp) >= n ? a = t : i = t + 1;
	}
	return i;
}
async function Pg(e, t, n, r = null) {
	let i = await e.getBlockNumber(), a = Number(n.lastScannedBlock ?? -1), o;
	if (a < 0) {
		let t = 0n;
		try {
			t = BigInt(await e.privacyCommitmentActivationTime());
		} catch {}
		o = await Ng(e, i, t);
	} else o = Math.max(0, a - 12);
	let s = n.notes || [];
	for (let t of s) {
		if (!t.nullifier) continue;
		let n = await e.privacyNullifierStatus(t.nullifier);
		if (n?.spent) {
			t.status = "spent";
			try {
				let e = Number(BigInt(n.spentHeight));
				Number.isSafeInteger(e) && e >= 0 && (o = Math.min(o, Math.max(0, e - 1)));
			} catch {}
		}
	}
	let c = s.filter((e) => !(e.source === "scan" && Number(e.createdBlock ?? -1) >= o));
	for (n.lastScannedBlock = o - 1; o <= i;) {
		let a = Math.min(i, o + 2047), s = await e.privacyShieldedOutputs(o, a);
		for (let n of s || []) {
			let r = kg(n, t);
			if (!r) continue;
			let i = await e.privacyCommitmentPath(r.commitment), a = await e.privacyNullifierStatus(r.nullifier);
			Mg(c, {
				id: `scan-${String(n.transactionHash).slice(2, 14)}-${Number(BigInt(n.outputIndex))}`,
				commitment: r.commitment,
				version: Number(r.version || 1),
				ownerSecret: r.ownerSecret || "",
				noteRandomness: r.noteRandomness,
				noteValueWei: r.noteValueWei,
				assetId: r.assetId,
				nullifier: r.nullifier,
				merklePath: (i.merklePath || []).map(String),
				merklePathIndex: (i.merklePathIndex || []).map((e) => BigInt(e).toString()),
				merkleRoot: i.root || "",
				createdTxHash: n.transactionHash,
				createdBlock: Number(BigInt(n.blockNumber)),
				source: "scan",
				status: a?.spent ? "spent" : i.found ? "available" : "pending"
			});
		}
		n.lastScannedBlock = a, r?.(a, i), o = a + 1;
	}
	for (let t of c) if (t.nullifier && ((await e.privacyNullifierStatus(t.nullifier))?.spent && (t.status = "spent"), t.status !== "pendingSpent" && t.status === "pending")) {
		let n = await e.privacyCommitmentPath(t.commitment);
		n?.found && (t.merklePath = (n.merklePath || []).map(String), t.merklePathIndex = (n.merklePathIndex || []).map((e) => BigInt(e).toString()), t.merkleRoot = n.root, t.status = "available");
	}
	return n.notes = c, n;
}
function Fg(e) {
	return (e || []).reduce((e, t) => t.status === "available" ? e + BigInt(t.noteValueWei) : e, 0n);
}
//#endregion
//#region vendor/api.js
var Ig = class {
	constructor(e) {
		this.provider = e, this.rpcUrl = e?._getConnection?.()?.url || e?.connection?.url || globalThis.location?.href || "";
	}
	async rkAdd(e, t = 0) {
		let n = [e];
		return t > 0 && n.push(t.toString()), await this.provider.send("rk_add", n);
	}
	async rkList() {
		return await this.provider.send("rk_list", []);
	}
	async rkStatus(e) {
		return e ? await this.provider.send("rk_status", [e]) : null;
	}
	async rkGetKingStats() {
		return await this.provider.send("rk_getKingStats", []);
	}
	async getKingInfo() {
		try {
			let e = await this.rkList();
			console.log("Kings from rk_list:", e);
			let t = null;
			try {
				t = await this.rkGetKingStats(), console.log("Stats from rk_getKingStats:", t);
			} catch (e) {
				console.warn("rk_getKingStats not available:", e.message);
			}
			let n = null, r = "—", i = "—", a = "—", o = "—", s = "—", c = "—", l = e ? e.length : 0, u = 0;
			if (e && e.length > 0) {
				u = e.filter((e) => e.registered).length;
				let t = e.find((e) => e.current), d = e.find((e) => e.next);
				r = t ? t.address : "—", a = d ? d.address : "—";
				for (let t of e) if (t.nextRotationHeight) {
					o = t.nextRotationHeight;
					break;
				}
				try {
					t && (n = await this.rkStatus(t.address), n && (i = n.mainKing || "—", c = n.rotationInterval || "—", s = n.blocksUntilRotation || "—", n.totalKings && (l = n.totalKings), n.registeredKings && (u = n.registeredKings)));
				} catch (e) {
					console.warn("rk_status failed:", e.message);
				}
			}
			return t && (t.totalKings && (l = t.totalKings), t.registeredKings && (u = t.registeredKings)), {
				status: n,
				kings: e || [],
				stats: t,
				currentKing: r,
				mainKing: i,
				nextKing: a,
				nextRotationHeight: o,
				blocksUntilRotation: s,
				rotationInterval: c,
				totalKings: l,
				registeredKings: u,
				currentBlock: 0
			};
		} catch (e) {
			return console.error("Error getting king info:", e), {
				status: null,
				kings: [],
				stats: null,
				currentKing: "—",
				mainKing: "—",
				nextKing: "—",
				nextRotationHeight: "—",
				blocksUntilRotation: "—",
				rotationInterval: "—",
				totalKings: 0,
				registeredKings: 0,
				currentBlock: 0
			};
		}
	}
	async getBlockNumber() {
		try {
			let e = await this.provider.send("eth_blockNumber", []);
			return parseInt(e, 16);
		} catch (e) {
			return console.error("getBlockNumber failed:", e), 0;
		}
	}
	async getBalance(e) {
		try {
			return await this.provider.send("eth_getBalance", [e, "latest"]);
		} catch (t) {
			console.warn("getBalance failed:", t.message);
			try {
				return await this.provider.send("eth_getBalance", [e, "pending"]);
			} catch (e) {
				return console.warn("getBalance with pending also failed:", e.message), "0x0";
			}
		}
	}
	async getGasPrice() {
		try {
			return await this.provider.send("eth_gasPrice", []);
		} catch (e) {
			return console.warn("getGasPrice failed:", e.message), "0x3b9aca00";
		}
	}
	async getTransactionCount(e, t = "pending") {
		return this.provider.send("eth_getTransactionCount", [e, t]);
	}
	async sendRawTransaction(e) {
		return this.provider.send("eth_sendRawTransaction", [e]);
	}
	async transactionBucket() {
		return this.provider.send("tkmprivacy_transactionBucket", []);
	}
	async beginTransactionBatch(e, t) {
		return this.provider.send("tkmprivacy_beginTransactionBatch", [V(e), t]);
	}
	async queueRawTransaction(e, t, n) {
		return this.provider.send("tkmprivacy_queueRawTransaction", [
			e,
			V(t),
			n
		]);
	}
	async transactionBatchStatus(e) {
		return this.provider.send("tkmprivacy_transactionBatchStatus", [e]);
	}
	async cancelTransactionBatch(e, t) {
		return this.provider.send("tkmprivacy_cancelTransactionBatch", [e, t]);
	}
	async exportPQAccount(e, t, n = t) {
		return this.provider.send("tkm_exportPQAccount", [
			e,
			t,
			n
		]);
	}
	async importLegacyKeyfileWithPassphrase(e, t) {
		let n = S(H(JSON.stringify(e)));
		return this.provider.send("tkm_importLegacyKeyfileWithPassphrase", [n, t]);
	}
	async preparePQMigrationWithPassphrase(e, t) {
		return this.provider.send("tkm_preparePQMigrationWithPassphrase", [e, t]);
	}
	async sendMigrationToPQWithPassphrase(e, t, n) {
		return this.provider.send("tkm_sendMigrationToPQWithPassphrase", [
			e,
			t,
			n
		]);
	}
	async pqMigrationGas(e) {
		return this.provider.send("tkm_pqMigrationGas", [e]);
	}
	async getMigrationGasPrice() {
		return this.provider.send("eth_gasPrice", []);
	}
	async accountAlgorithm(e) {
		return this.provider.send("tkm_accountAlgorithm", [e]);
	}
	async privacyCommitmentActive() {
		return this.provider.send("tkmprivacy_commitmentActive", []);
	}
	async privacyCommitmentActivationTime() {
		return this.provider.send("tkmprivacy_commitmentActivationTime", []);
	}
	async shieldedV2ActivationTime() {
		return this.provider.send("tkmprivacy_shieldedV2ActivationTime", []);
	}
	async shieldedV2Active() {
		return this.provider.send("tkmprivacy_shieldedV2Active", []);
	}
	async shieldedGasSponsorActivationTime() {
		return this.provider.send("tkmprivacy_shieldedGasSponsorActivationTime", []);
	}
	async shieldedGasSponsorActive() {
		return this.provider.send("tkmprivacy_shieldedGasSponsorActive", []);
	}
	async privacyDefaults() {
		return this.provider.send("tkmprivacy_defaults", []);
	}
	async privacyShieldedOutputs(e, t) {
		return this.provider.send("tkmprivacy_shieldedOutputs", [V(e), V(t)]);
	}
	async privacyCommitmentPath(e) {
		return this.provider.send("tkmprivacy_commitmentPath", [e]);
	}
	async privacyNullifierStatus(e) {
		return this.provider.send("tkmprivacy_nullifierStatus", [e]);
	}
	async privacyStatus(e) {
		return this.provider.send("tkmprivacy_status", [e]);
	}
	async getTransactionReceipt(e) {
		try {
			return await this.provider.send("eth_getTransactionReceipt", [e]);
		} catch (e) {
			return console.warn("getTransactionReceipt failed:", e.message), null;
		}
	}
	async estimateGas(e) {
		try {
			return await this.provider.send("eth_estimateGas", [e]);
		} catch (e) {
			return console.warn("estimateGas failed:", e.message), "0x5208";
		}
	}
	async getBlock(e, t = !0) {
		try {
			let n = typeof e == "number" ? "0x" + e.toString(16) : e;
			return await this.provider.send("eth_getBlockByNumber", [n, t]);
		} catch (e) {
			return console.warn("getBlock failed:", e.message), null;
		}
	}
	async getTransactionByHash(e) {
		try {
			return await this.provider.send("eth_getTransactionByHash", [e]);
		} catch (e) {
			return console.warn("getTransactionByHash failed:", e.message), null;
		}
	}
	async getIndexedTransactionHistory(e, t = 100) {
		let n = `${new URL(this.rpcUrl || globalThis.location?.href || "").origin}/api/history/${encodeURIComponent(e)}?limit=${encodeURIComponent(t)}`, r = await fetch(n, {
			method: "GET",
			headers: { Accept: "application/json" },
			cache: "no-store"
		});
		if (!r.ok) throw Error(`history indexer returned HTTP ${r.status}`);
		let i = await r.json();
		return Array.isArray(i.transactions) ? i.transactions : [];
	}
	async getTransactionHistory(e, t = 0, n = "latest") {
		try {
			let r = [], i = n;
			n === "latest" && (i = await this.getBlockNumber());
			let a = Math.max(0, t), o = i - a, s = a;
			o > 1e3 && (s = i - 1e3), console.log(`Scanning blocks from ${s} to ${i}`);
			for (let t = s; t <= i; t++) try {
				let n = await this.getBlock(t, !0);
				if (!n || !n.transactions) continue;
				for (let t of n.transactions) {
					let i = t.from && t.from.toLowerCase() === e.toLowerCase(), a = t.to && t.to.toLowerCase() === e.toLowerCase();
					if (i || a) {
						let e = null;
						try {
							e = await this.getTransactionReceipt(t.hash);
						} catch {}
						let a = i ? "out" : "in";
						r.push({
							hash: t.hash,
							from: t.from,
							to: t.to || "Contract Creation",
							value: t.value ? nt(t.value) : "0",
							direction: a,
							blockNumber: n.number,
							timestamp: n.timestamp,
							status: e ? e.status === "0x1" ? "confirmed" : "failed" : "pending",
							gasUsed: e ? e.gasUsed : "—",
							gasPrice: t.gasPrice ? nt(t.gasPrice) : "—",
							nonce: t.nonce
						});
					}
				}
			} catch {}
			try {
				let t = await this.provider.send("txpool_content", []);
				if (t && t.pending) {
					for (let [n, i] of Object.entries(t.pending)) if (n.toLowerCase() === e.toLowerCase()) for (let [e, t] of Object.entries(i)) r.some((e) => e.hash === t.hash) || r.push({
						hash: t.hash,
						from: t.from,
						to: t.to || "Contract Creation",
						value: t.value ? nt(t.value) : "0",
						direction: "out",
						blockNumber: "pending",
						timestamp: null,
						status: "pending",
						gasUsed: "—",
						gasPrice: t.gasPrice ? nt(t.gasPrice) : "—",
						nonce: parseInt(e)
					});
				}
			} catch (e) {
				console.warn("txpool_content not available:", e.message);
			}
			return r.sort((e, t) => e.blockNumber === "pending" ? -1 : t.blockNumber === "pending" ? 1 : t.blockNumber - e.blockNumber), r;
		} catch (e) {
			return console.error("Error getting transaction history:", e), [];
		}
	}
	async getChainId() {
		try {
			let e = await this.provider.send("eth_chainId", []);
			return parseInt(e, 16);
		} catch (e) {
			return console.warn("getChainId failed:", e.message), 8979;
		}
	}
	async domainRpc(e, t = []) {
		return this.provider.send("tkmdomain_" + e, t);
	}
	async domainStatus() {
		return this.domainRpc("status");
	}
	async domainSuperAddress() {
		return this.domainRpc("superAddress");
	}
	async domainClaimSuper() {
		return this.domainRpc("claimSuper");
	}
	async domainRegistrationFee() {
		return this.domainRpc("registrationFee");
	}
	async domainSubscriberUnitPrice() {
		return this.domainRpc("subscriberUnitPrice");
	}
	async domainQuote(e) {
		return this.domainRpc("quote", [V(e)]);
	}
	async domainOperator(e, t, n, r = "") {
		return r ? this.domainRpc("operatorWithPayout", [
			V(e),
			String(t),
			n,
			r
		]) : this.domainRpc("operator", [
			V(e),
			String(t),
			n
		]);
	}
	async domainSetPayout(e, t) {
		return this.domainRpc("setPayout", [e, t]);
	}
	async domainBuy(e, t) {
		return this.domainRpc("buy", [e, t]);
	}
	async domainExpand(e, t, n) {
		return this.domainRpc("expand", [
			e,
			V(t),
			String(n)
		]);
	}
	async domainGet(e) {
		return this.domainRpc("domain", [e]);
	}
	async domainList() {
		return this.domainRpc("domains");
	}
	async domainHash(e) {
		return this.domainRpc("domainHash", [e]);
	}
	async domainMailboxHash(e, t) {
		return this.domainRpc("mailboxHash", [e, t]);
	}
	async domainRegistration(e) {
		return this.domainRpc("registration", [e]);
	}
	async domainMailbox(e) {
		return this.domainRpc("mailbox", [e]);
	}
	async domainMailboxes(e = "") {
		return this.domainRpc("mailboxes", [e]);
	}
	async domainPending() {
		return this.domainRpc("pending");
	}
	async domainSync() {
		return this.domainRpc("sync");
	}
	async emailRpc(e, t = []) {
		return this.provider.send("emailvm_" + e, t);
	}
	async emailStatus() {
		return this.emailRpc("status");
	}
	async emailPublishKey(e, t) {
		return this.emailRpc("publishKey", [e, t]);
	}
	async emailKey(e) {
		return this.emailRpc("key", [e]);
	}
	async emailSend(e, t, n, r) {
		return this.emailRpc("send", [
			e,
			t,
			n,
			r
		]);
	}
	async emailInbox(e) {
		return this.emailRpc("inbox", [e]);
	}
	async emailOutbox(e) {
		return this.emailRpc("outbox", [e]);
	}
	async emailInboxPage(e, t = 0, n = 50) {
		return this.emailRpc("inboxPage", [
			e,
			V(t),
			V(n)
		]);
	}
	async emailOutboxPage(e, t = 0, n = 50) {
		return this.emailRpc("outboxPage", [
			e,
			V(t),
			V(n)
		]);
	}
	async emailMessage(e) {
		return this.emailRpc("message", [e]);
	}
	async phoneRpc(e, t = []) {
		return this.provider.send("tkmphone_" + e, t);
	}
	async phoneStatus() {
		return this.phoneRpc("status");
	}
	async phoneBuckets() {
		return this.phoneRpc("buckets");
	}
	async phoneRegisteredNumbers() {
		return this.phoneRpc("registeredNumbers");
	}
	async phoneListOperators() {
		return this.phoneRpc("listOperators");
	}
	async phonePendingOperatorApprovals(e = 2e4) {
		return this.phoneRpc("pendingOperatorApprovals", [e]);
	}
	async phoneNumber(e) {
		return this.phoneRpc("number", [e]);
	}
	async phoneDeviceKeys(e) {
		return this.phoneRpc("deviceKeys", [e]);
	}
	async phoneOpenBucketHash(e, t) {
		return this.phoneRpc("openBucketHash", [e, t]);
	}
	async phoneOpenBucket(e, t, n) {
		return this.phoneRpc("openBucket", [
			e,
			t,
			n
		]);
	}
	async phoneOperatorGrantHash(e, t, n, r) {
		return this.phoneRpc("operatorGrantHash", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneRegisterOperatorKey(e, t, n, r, i, a) {
		return this.phoneRpc("registerOperatorKey", [
			e,
			t,
			n,
			r,
			i,
			a
		]);
	}
	async phoneSellNumber(e, t, n, r, i) {
		return this.phoneRpc("sellNumber", [
			e,
			t,
			n,
			r,
			i
		]);
	}
	async phoneDeviceKeySigningHash(e, t, n) {
		return this.phoneRpc("deviceKeySigningHash", [
			e,
			t,
			n
		]);
	}
	async phoneDeviceKeySigningHashV2(e, t, n, r) {
		return this.phoneRpc("deviceKeySigningHashV2", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneRegisterDeviceKey(e, t, n, r) {
		return this.phoneRpc("registerDeviceKey", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneRegisterDeviceKeyV2(e, t, n, r, i) {
		return this.phoneRpc("registerDeviceKeyV2", [
			e,
			t,
			n,
			r,
			i
		]);
	}
	async phoneEncryptPayload(e, t, n, r) {
		return this.phoneRpc("encryptPayload", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneDecryptPayload(e, t, n, r) {
		return this.phoneRpc("decryptPayload", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneSendMessageSigningHash(e, t, n, r) {
		return this.phoneRpc("sendMessageSigningHash", [
			e,
			t,
			n,
			r
		]);
	}
	async phoneSendEncryptedMessage(e, t, n, r, i) {
		return this.phoneRpc("sendEncryptedMessage", [
			e,
			t,
			n,
			r,
			i
		]);
	}
	async phoneMessagesForNumber(e) {
		return this.phoneRpc("messagesForNumber", [e]);
	}
	async phoneNotifications(e) {
		return this.phoneRpc("notifications", [e]);
	}
	async phoneTransferNumberSigningHash(e, t) {
		return this.phoneRpc("transferNumberSigningHash", [e, t]);
	}
	async phoneTransferNumber(e, t, n) {
		return this.phoneRpc("transferNumber", [
			e,
			t,
			n
		]);
	}
}, Lg = !1;
function Rg() {
	if (Lg) throw Error("Another wallet operation is in progress. Wait for it to finish.");
	return Lg = !0, () => {
		Lg = !1;
	};
}
//#endregion
//#region shield2-send.js
var zg = 3000000n, Bg = (1n << 64n) - 1n;
function Vg(e, t, n, r, i) {
	let a = BigInt(t), o = BigInt(r);
	if (a <= 0n) throw Error("Amount must be positive.");
	let s = [];
	for (let t of e.filter((e) => e.status === "available" && Number(e.version) === 2 && BigInt(e.assetId) === 1n)) {
		if (!a) break;
		let e = i && o < n ? n - o : 0n;
		if (!i && o < n) break;
		let r = BigInt(t.noteValueWei) - e;
		if (r <= 0n) continue;
		let c = a < r ? a : r;
		s.push({
			note: t,
			value: c,
			sponsor: e
		}), o -= n - e, a -= c;
	}
	for (; a > 0n;) {
		let e = a < Bg ? a : Bg;
		if (o < e + n) throw Error("Insufficient spendable TKM including network gas.");
		s.push({
			note: null,
			value: e,
			sponsor: 0n
		}), o -= e + n, a -= e;
	}
	return s;
}
function Hg(e, t, n, r, i) {
	let a = BigInt(t), o = BigInt(r);
	if (a <= 0n) throw Error("Amount must be positive.");
	let s = [];
	for (let t of e.filter((e) => e.status === "available" && Number(e.version) === 2 && BigInt(e.assetId) === 1n)) {
		if (!a || !i && o < n) break;
		let e = o < n ? n - o : 0n, r = BigInt(t.noteValueWei) + n - e, c = a < r ? a : r;
		c <= n || (s.push({
			note: t,
			value: c - n,
			sponsor: e,
			fee: n,
			budget: c
		}), o -= n - e, a -= c);
	}
	for (; a > 0n;) {
		let e = a < Bg + n ? a : Bg + n;
		if (e <= n || o < e) throw Error("Insufficient payout amount or spendable funds after network gas.");
		s.push({
			note: null,
			value: e - n,
			sponsor: 0n,
			fee: n,
			budget: e
		}), o -= e, a -= e;
	}
	return s;
}
function Ug(e, { nonce: t, gasPrice: n, part: r, address: i, recipient: a }) {
	let o = e.transaction;
	if (jg(o, {
		chainId: 8979,
		value: r.note ? 0n : r.value
	}), BigInt(o.nonce) !== BigInt(t) || BigInt(o.gas) !== zg || BigInt(o.gasFeeCap) !== n || BigInt(o.gasTipCap) !== n || !Array.isArray(o.accessList) || o.accessList.length) throw Error("Proof builder changed transaction fees or nonce.");
	if (bg(o, e) !== r.sponsor) throw Error("Proof builder changed gas sponsorship.");
	let s = [{
		index: 0,
		recipient: a.address,
		assetId: 1n,
		valueWei: r.value
	}];
	if (r.note) {
		if (String(e.spentNullifier).toLowerCase() !== r.note.nullifier.toLowerCase()) throw Error("Proof builder changed the input note.");
		let t = BigInt(r.note.noteValueWei) - r.value - r.sponsor;
		t > 0n && s.push({
			index: 1,
			recipient: i,
			assetId: 1n,
			valueWei: t
		});
	}
	vg(o, e, s);
	let c = Je(_(o.data).slice(10));
	if (c[6] && c[6] !== "0x" && BigInt(c[6]) !== 0n || c[1].length !== +!!r.note) throw Error("Proof builder changed the spend or withdrawal.");
	if (r.note && S(c[1][0][0]).toLowerCase() !== r.note.nullifier.toLowerCase()) throw Error("Proof envelope contains the wrong nullifier.");
	return o;
}
async function Wg({ keystore: e, password: t, intent: n, rpcURL: r, proverURL: i, proverToken: a, onProgress: o, beforePart: s, onSubmitted: c, prepareOnly: l = !1, proverFetch: u, recipientPaysGas: d = !1, rpcToken: f = "" }) {
	let p = Rg(), m, h, g, _ = [];
	try {
		let p = new URL(r), v = new URL(i);
		if (!["https:", "http:"].includes(p.protocol)) throw Error("Enter an HTTP(S) TKM RPC URL.");
		if (v.protocol !== "https:" && !(v.protocol === "http:" && ([
			"127.0.0.1",
			"localhost",
			"[::1]"
		].includes(v.hostname) || typeof window < "u" && v.origin === window.location.origin))) throw Error("The configured proof builder must use HTTPS.");
		let y = Dg(n.from, 8979), b = Dg(n.deposit_address || n.recipient_address, 8979);
		if (y.version !== 2 || b.version !== 2) throw Error("TKM requires shield2 payment codes.");
		o("Unlocking keyfile locally...");
		let x = Zm(e);
		m = await sh(x, t);
		let C = K("0x" + String(x.address).replace(/^0x/i, ""));
		if (h = wg(m, C, 8979), C !== y.address || S(h.viewPublicKey) !== S(y.viewPublicKey)) throw Error("Keyfile does not match the source payment code.");
		let w = new Oe(r);
		if (f && w.setHeader("X-GUI-Token", f), g = new Vd(w), BigInt(await g.send("eth_chainId", [])) !== 8979n) throw Error("RPC is not TKM chain 8979.");
		let T = new Ig(g);
		if (!await T.shieldedV2Active()) throw Error("Shielded V2 is not active.");
		let E = new Ag(i, a, u), D = await E.health();
		if (!D.ok || !D.hasProvingKeyV2) throw Error(D.startupError || "Proof builder is not ready.");
		let O = {
			lastScannedBlock: -1,
			notes: []
		};
		await Pg(T, h, O, (e, t) => o(`Scanning notes: ${e} / ${t}`));
		let k = BigInt(await g.send("eth_gasPrice", [])), A = BigInt(await g.send("eth_getBalance", [C, "latest"])), j = (d ? Hg : Vg)(O.notes, tt(n.amount, 18), zg * k, A, await T.shieldedGasSponsorActive());
		for (let e = 0; e < j.length; e++) {
			let t = j[e], n = await T.getTransactionCount(C, "latest");
			if (BigInt(await T.getTransactionCount(C, "pending")) !== BigInt(n)) throw Error("Source wallet has a pending transaction. Wait for confirmation before retrying.");
			let r = BigInt(await g.send("eth_getBalance", [C, "latest"]));
			t.note && (t.sponsor = r < zg * k ? zg * k - r : 0n), o(`Building proof ${e + 1} of ${j.length}...`);
			let i = {
				requestId: `exchange-${S($n(16)).slice(2)}`,
				from: C,
				to: b.address,
				amountWei: t.value.toString(),
				recipientViewKey: S(b.viewPublicKey),
				nonce: n,
				gasPriceWei: k.toString()
			}, a;
			if (t.note) {
				if ((await T.privacyNullifierStatus(t.note.nullifier))?.spent) throw Error("A selected note was spent. Refresh before retrying.");
				let e = await T.privacyCommitmentPath(t.note.commitment);
				if (!e.found) throw Error("Selected note is no longer confirmed.");
				let n = {
					...t.note,
					merkleRoot: e.root,
					merklePath: e.merklePath.map(String),
					merklePathIndex: e.merklePathIndex.map((e) => BigInt(e).toString())
				};
				a = await E.buildTransfer({
					...i,
					note: n,
					changeViewKey: S(h.viewPublicKey)
				});
			} else a = await E.buildDeposit({
				...i,
				assetId: "1",
				ownerSecret: BigInt(C).toString()
			});
			let u = Ug(a, {
				nonce: n,
				gasPrice: k,
				part: t,
				address: C,
				recipient: b
			}), f = l ? null : await s(et(t.value, 18), j.length);
			o(`Signing and broadcasting part ${e + 1} locally...`);
			let p = uh(u, m), v = W(p.rawTransaction);
			if (l) return {
				hash: v,
				raw: p.rawTransaction,
				amountWei: t.value.toString(),
				feeWei: d ? t.fee.toString() : "0",
				gasPriceWei: k.toString()
			};
			if (_.push(v), await c(f, v), await T.sendRawTransaction(p.rawTransaction), e + 1 < j.length) {
				o(`Waiting for confirmation: ${v}`);
				let e = !1;
				for (let t = 0; t < 120; t++) {
					let t = await T.getTransactionReceipt(v);
					if (t) {
						if (BigInt(t.status) !== 1n) throw Error(`Transaction reverted: ${v}`);
						e = !0;
						break;
					}
					await new Promise((e) => setTimeout(e, 2e3));
				}
				if (!e) throw Error("Confirmation timed out. Check submitted hashes before retrying.");
			}
		}
		return _;
	} catch (e) {
		throw _.length && (e.message += ` Submitted or attempted hashes: ${_.join(", ")}. Check these before retrying.`), e;
	} finally {
		m?.fill(0), Tg(h), g?.destroy(), p();
	}
}
//#endregion
//#region shield3.js
var Gg = 5000000n * 10n ** 18n;
function Kg(e) {
	let t = tt(String(e), 18);
	if (t <= 0n || t > Gg) throw Error("Shield3 sends must be greater than zero and at most 5,000,000 TKM.");
	return t;
}
function qg(e) {
	if (!Array.isArray(e) || e.length < 1 || e.length > 3) throw Error("Choose one to three Shield3 recipients.");
	let t = 0n;
	return {
		requests: e.map((e) => {
			if (!e.recipient_address?.startsWith("tkmshield3.")) throw Error("Every recipient requires a Shield3 receiving address.");
			let n = Kg(e.amount);
			if (t += n, t > Gg) throw Error("The combined payment amount must not exceed 5,000,000 TKM.");
			return {
				recipient: e.recipient_address,
				amountWei: n.toString()
			};
		}),
		total: t
	};
}
async function Jg({ rpcURL: e, rpcToken: t }) {
	let n = await (await fetch(e, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"X-GUI-Token": t
		},
		body: JSON.stringify({
			jsonrpc: "2.0",
			id: 1,
			method: "tkmprivacy_shieldedV3Status",
			params: []
		})
	})).json();
	if (n.error) {
		if (n.error.code === -32601) return { active: !1 };
		throw Error(n.error.message);
	}
	return n.result;
}
async function Yg(e, { walletURL: t, rpcToken: n }) {
	return Xg("validate", { recipient: e }, {
		walletURL: t,
		rpcToken: n
	});
}
async function Xg(e, t, { walletURL: n, rpcToken: r }) {
	let i = n || new URL("/shield3", globalThis.location.href).href, a = new URL(i, globalThis.location?.href);
	if (![
		"127.0.0.1",
		"localhost",
		"[::1]"
	].includes(a.hostname) || a.origin !== globalThis.location.origin) throw Error("Shield3 private operations require your local wallet.");
	let o = await fetch(a.href.replace(/\/$/, "") + "/" + e, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"X-GUI-Token": r
		},
		body: JSON.stringify(t)
	}), s = await o.json();
	if (!o.ok || s.error) throw Error(s.error || "Shield3 wallet operation failed.");
	return s;
}
async function Zg(e, t, n = {}) {
	let r = Zm(t.keystore), i = await sh(r, t.password);
	try {
		return await Xg(e, {
			seed: S(i),
			stamp: r.shield3Stamp,
			account: K("0x" + r.address.replace(/^0x/, "")),
			...n
		}, t);
	} finally {
		i.fill(0);
	}
}
function Qg(e) {
	return Zg("identity", e);
}
function $g(e) {
	return Zg("viewkeys", e, { scope: e.scope || "incoming" });
}
function e_(e) {
	return Xg("view-scan", { view: e.view }, e);
}
function t_(e) {
	return Xg("view-stamp", { stampDisclosure: e.stampDisclosure }, e);
}
function n_(e) {
	return Xg("fetch-relay-offer", {
		relayURL: e.relayURL,
		requestId: e.requestId
	}, e);
}
function r_(e) {
	return Zg("scan", e);
}
async function i_(e) {
	let t = Rg();
	try {
		let { requests: t } = qg(e.intent.payments || [e.intent]), n = e.requestId || S($n(16));
		e.onProgress?.("Building the private Shield3 proof…");
		let r;
		if (e.relayURL) {
			let i = await Zg("prepare-relay", e, {
				payments: t,
				relay: e.relayOffer,
				relayURL: e.relayURL,
				requestId: n
			});
			e.onPrepared?.(i), e.onProgress?.("Submitting the saved payment to the shared relay…");
			for (let t = 0; t < 3; t++) {
				try {
					if (r = await c_({
						...e,
						stage: "retry",
						requestId: n
					}), !r.submissionUncertain || t === 2) break;
				} catch (e) {
					if (t === 2) throw e;
				}
				e.onProgress?.("Relay submission uncertain. Retrying the same saved transaction…"), await new Promise((e) => setTimeout(e, 2e3));
			}
		} else r = await Zg("send", e, {
			payments: t,
			requestId: n
		});
		return e.onSubmitted?.(0, r.transactionHash), (r.submissionError || r.submissionUncertain) && e.onProgress?.("Submission status is uncertain. Check this hash before retrying: " + r.transactionHash), [r.transactionHash];
	} finally {
		t();
	}
}
async function a_(e) {
	let t = Rg();
	try {
		return await Zg("shield", e, {
			amountWei: Kg(e.amount).toString(),
			requestId: e.requestId || S($n(16))
		});
	} finally {
		t();
	}
}
async function o_(e) {
	let t = Rg();
	try {
		return await Zg("register-stamp", e, { requestId: e.requestId });
	} finally {
		t();
	}
}
async function s_(e) {
	let t = {
		offer: "stamp-offer",
		authorize: "authorize-stamp",
		review: "review-sponsorship",
		submit: "sponsor-stamp"
	}[e.stage];
	if (!t) throw Error("Choose a stamp sponsorship step.");
	let n = Rg();
	try {
		return await Zg(t, e, {
			recipient: e.recipient,
			sponsorship: e.sponsorship,
			requestId: e.requestId
		});
	} finally {
		n();
	}
}
async function c_(e) {
	let t = {
		offer: "relay-offer",
		prepare: "prepare-relay",
		review: "review-relay",
		submit: "submit-relay",
		status: "relay-status",
		retry: "submit-relay-draft"
	}[e.stage];
	if (!t) throw Error("Choose a relay step.");
	if (e.stage === "status" || e.stage === "retry") return Xg(t, {
		account: e.account || K("0x" + Zm(e.keystore).address.replace(/^0x/, "")),
		requestId: e.requestId
	}, e);
	let n = Rg();
	try {
		return await Zg(t, e, {
			relay: e.relay,
			relayTransaction: e.transaction,
			recipient: e.recipient,
			amountWei: e.amount ? Kg(e.amount).toString() : void 0,
			requestId: e.requestId
		});
	} finally {
		n();
	}
}
function l_(e) {
	return Xg("review-relay-offer", { relay: e.relay }, e);
}
async function u_(e) {
	return e.stage === "verify" ? Xg("verify-disclosure", {
		disclosure: e.disclosure,
		capsule: e.capsule,
		auditKey: e.auditKey
	}, e) : Zg(e.stage === "key" ? "disclosure-key" : "export-disclosure", e, {
		transactionHash: e.transactionHash,
		outputIndex: e.outputIndex,
		auditPublicKey: e.auditPublicKey
	});
}
//#endregion
//#region node_modules/@noble/post-quantum/ml-kem.js
var d_ = 256, f_ = 3329, p_ = /* @__PURE__ */ Tm({
	N: d_,
	Q: f_,
	F: 3303,
	ROOT_OF_UNITY: 17,
	newPoly: (e) => new Uint16Array(e),
	brvBits: 7,
	isKyber: !0
}), m_ = /* @__PURE__ */ Object.freeze({
	512: Object.freeze({
		N: d_,
		Q: f_,
		K: 2,
		ETA1: 3,
		ETA2: 2,
		du: 10,
		dv: 4,
		RBGstrength: 128
	}),
	768: Object.freeze({
		N: d_,
		Q: f_,
		K: 3,
		ETA1: 2,
		ETA2: 2,
		du: 10,
		dv: 4,
		RBGstrength: 192
	}),
	1024: Object.freeze({
		N: d_,
		Q: f_,
		K: 4,
		ETA1: 2,
		ETA2: 2,
		du: 11,
		dv: 5,
		RBGstrength: 256
	})
}), h_ = (e) => {
	if (e >= 12) return {
		encode: (e) => e,
		decode: (e) => e >= f_ ? e - f_ : e
	};
	let t = 2 ** (e - 1);
	return {
		encode: (t) => ((t << e) + f_ / 2) / f_,
		decode: (n) => n * f_ + t >>> e
	};
}, g_ = (e) => p_.bitsCoder(e, e === 12 ? {
	encode: (e) => e,
	decode: (e) => e >= f_ ? e - f_ : e
} : {
	encode: (e) => e,
	decode: (e) => e
}), __ = (e) => e === 12 ? g_(12) : p_.bitsCoder(e, h_(e));
function v_(e, t) {
	let n = e, r = t;
	for (let e = 0; e < d_; e++) {
		let t = n[e] + r[e];
		n[e] = t >= f_ ? t - f_ : t;
	}
}
function y_(e, t) {
	let n = e, r = t;
	for (let e = 0; e < d_; e++) {
		let t = n[e] - r[e];
		n[e] = t < 0 ? t + f_ : t;
	}
}
function b_(e, t, n, r, i) {
	return {
		c0: p_.mod(p_.mod(t * r) * i + e * n),
		c1: p_.mod(e * r + t * n)
	};
}
function x_(e, t) {
	let n = e, r = t;
	for (let e = 0; e < d_ / 2; e++) {
		let t = p_.nttZetas[64 + (e >> 1)];
		e & 1 && (t = -t);
		let { c0: i, c1: a } = b_(n[2 * e + 0], n[2 * e + 1], r[2 * e + 0], r[2 * e + 1], t);
		n[2 * e + 0] = i, n[2 * e + 1] = a;
	}
	return n;
}
function S_(e) {
	let t = e, n = new Uint16Array(d_);
	for (let e = 0; e < d_;) {
		let r = t();
		if (r.length % 3) throw Error("SampleNTT: unaligned block");
		for (let t = 0; e < d_ && t + 3 <= r.length; t += 3) {
			let i = (r[t + 0] >> 0 | r[t + 1] << 8) & 4095, a = (r[t + 1] >> 4 | r[t + 2] << 4) & 4095;
			i < f_ && (n[e++] = i), e < d_ && a < f_ && (n[e++] = a);
		}
	}
	return n;
}
var C_ = (e, t) => {
	let n = new Uint16Array(d_), r = lp(e);
	mp(r);
	let i = 0;
	for (let e = 0, a = 0, o = 0, s = 0; e < r.length; e++) {
		let c = r[e];
		for (let e = 0; e < 32; e++) o += c & 1, c >>= 1, i += 1, i === t ? (s = o, o = 0) : i === 2 * t && (n[a++] = p_.mod(s - o), o = 0, i = 0);
	}
	if (mp(r), i) throw Error(`sampleCBD: leftover bits: ${i}`);
	return n;
};
function w_(e, t, n, r) {
	return C_(e(r * d_ / 4, t, n), r);
}
var T_ = (e) => {
	let { K: t, PRF: n, XOF: r, HASH512: i, ETA1: a, ETA2: o, du: s, dv: c } = e, l = __(1), u = __(c), d = __(s), f = gm("publicKey", _m(__(12), t), 32), p = _m(__(12), t), m = gm("ciphertext", _m(d, t), u), h = gm("seed", 32, 32), g = (e, r, i, s) => {
		let c = [];
		for (let e = 0; e < t; e++) c.push(p_.NTT.encode(w_(n, s, e, a)));
		let u = new Uint16Array(d_), d = [];
		for (let i = 0; i < t; i++) {
			let a = w_(n, s, t + i, o), l = new Uint16Array(d_);
			for (let e = 0; e < t; e++) v_(l, x_(r(i, e), c[e]));
			v_(a, p_.NTT.decode(l)), d.push(a), v_(u, x_(e[i], c[i])), vm(l);
		}
		let f = w_(n, s, 2 * t, o);
		v_(f, p_.NTT.decode(u));
		let p = l.decode(i);
		return v_(p, f), vm(e, c, u, f), m.encode([d, p]);
	};
	return {
		secretCoder: p,
		lengths: {
			secretKey: p.bytesLen,
			publicKey: f.bytesLen,
			cipherText: m.bytesLen
		},
		keygen: (e) => {
			sm(e, 32, "seed");
			let o = /* @__PURE__ */ new Uint8Array(33);
			o.set(e), o[32] = t;
			let s = i(o), [c, l] = h.decode(s), u = [], d = [];
			for (let e = 0; e < t; e++) u.push(p_.NTT.encode(w_(n, l, e, a)));
			let m = r(c);
			for (let e = 0; e < t; e++) {
				let r = p_.NTT.encode(w_(n, l, t + e, a));
				for (let n = 0; n < t; n++) v_(r, x_(S_(m.get(n, e)), u[n]));
				d.push(r);
			}
			m.clean();
			let g = {
				publicKey: f.encode([d, c]),
				secretKey: p.encode(u)
			};
			return vm(c, l, u, d, o, s), g;
		},
		encrypt: (e, t, n) => {
			let [i, a] = f.decode(e), o = r(a), s = g(i, (e, t) => S_(o.get(e, t)), t, n);
			return o.clean(), s;
		},
		prepare: (e) => {
			let [n, i] = f.decode(e), a = r(i), o = [];
			for (let e = 0; e < t; e++) for (let n = 0; n < t; n++) o.push(S_(a.get(e, n)));
			return a.clean(), {
				encrypt: (e, r) => g(n.map((e) => e.slice()), (e, n) => o[e * t + n].slice(), e, r),
				clean: () => vm(n, o)
			};
		},
		decrypt: (e, n) => {
			let [r, i] = m.decode(e), a = p.decode(n), o = new Uint16Array(d_);
			for (let e = 0; e < t; e++) v_(o, x_(a[e], p_.NTT.encode(r[e])));
			return y_(i, p_.NTT.decode(o)), vm(o, a, r), l.encode(i);
		}
	};
};
function E_(e) {
	let t = e, n = T_(t), { HASH256: r, HASH512: i, KDF: a } = t, { secretCoder: o, lengths: s } = n, c = gm("secretKey", s.secretKey, s.publicKey, 32, 32), l = (e, n) => {
		let r = e.subarray(0, 384 * t.K), i = o.encode(o.decode(fm(r))), a = dm(i, r);
		if (vm(i), !a) throw Error(`ML-KEM.${n}: wrong publicKey modulus`);
	}, u = Object.freeze({
		...s,
		seed: 64,
		msg: 32,
		msgRand: 32,
		secretKey: c.bytesLen
	});
	return Object.freeze({
		info: Object.freeze({ type: "ml-kem" }),
		lengths: u,
		keygen: (e = cm(64)) => {
			sm(e, 64, "seed");
			let { publicKey: t, secretKey: i } = n.keygen(e.subarray(0, 32)), a = r(t), o = c.encode([
				i,
				t,
				a,
				e.subarray(32)
			]);
			return vm(i, a), {
				publicKey: t,
				secretKey: o
			};
		},
		getPublicKey: (e) => {
			let [t, n, r, i] = c.decode(e);
			return Uint8Array.from(n);
		},
		encapsulate: (e, t = cm(32)) => {
			sm(e, s.publicKey, "publicKey"), sm(t, 32, "message"), l(e, "encapsulate");
			let a = i.create().update(t).update(r(e)).digest(), o = n.encrypt(e, t, a.subarray(32, 64));
			return vm(a.subarray(32)), {
				cipherText: o,
				sharedSecret: a.subarray(0, 32)
			};
		},
		decapsulate: (e, t) => {
			sm(t, c.bytesLen, "secretKey"), sm(e, s.cipherText, "cipherText");
			let o = c.bytesLen - 96, l = o + 32;
			if (!dm(r(t.subarray(o / 2, l)), t.subarray(l, l + 32))) throw Error("invalid secretKey: hash check failed");
			let [u, d, f, p] = c.decode(t), m = n.decrypt(e, u), h = i.create().update(m).update(f).digest(), g = h.subarray(0, 32), _ = n.encrypt(d, m, h.subarray(32, 64)), v = dm(e, _), y = a.create({ dkLen: 32 }).update(p).update(e).digest();
			return vm(m, _, h.subarray(32), v ? y : g), v ? g : y;
		},
		prepare: (e) => {
			sm(e, s.publicKey, "publicKey"), l(e, "prepare");
			let t = fm(e), o = r(t), u = n.prepare(t);
			return Object.freeze({
				publicKey: t,
				encapsulate: (e = cm(32)) => {
					sm(e, 32, "message");
					let t = i.create().update(e).update(o).digest(), n = u.encrypt(e, t.subarray(32, 64));
					return vm(t.subarray(32)), {
						cipherText: n,
						sharedSecret: t.subarray(0, 32)
					};
				},
				decapsulate: (e, r) => {
					sm(r, c.bytesLen, "secretKey"), sm(e, s.cipherText, "cipherText");
					let [l, d, f, p] = c.decode(r);
					if (!dm(d, t) || !dm(f, o)) throw Error("ML-KEM.decapsulate: secretKey does not match prepared publicKey");
					let m = n.decrypt(e, l), h = i.create().update(m).update(o).digest(), g = h.subarray(0, 32), _ = u.encrypt(m, h.subarray(32, 64)), v = dm(e, _), y = a.create({ dkLen: 32 }).update(p).update(e).digest();
					return vm(m, _, h.subarray(32), v ? y : g), v ? g : y;
				},
				clean: u.clean
			});
		}
	});
}
function D_(e, t, n) {
	return em.create({ dkLen: e }).update(t).update(new Uint8Array([n])).digest();
}
var O_ = {
	HASH256: Xp,
	HASH512: Zp,
	KDF: em,
	XOF: Dm,
	PRF: D_
}, k_ = /* @__PURE__ */ ((e) => E_({
	...O_,
	...e
}))(m_[1024]), A_ = new TextEncoder().encode("TKPQ"), j_ = new TextEncoder().encode("TKM_PHONE_ANTARTICAL_CONTEXT_V1"), M_ = new TextEncoder().encode("TKM_SHIELD3_ENCRYPTION_TRANSCRIPT_V1"), N_ = new TextEncoder().encode("TKM_SHIELD3_XCHACHA20POLY1305_KEY_V1"), P_ = new TextEncoder().encode("TKM_PHONE_ANTARTICAL_DEVICE_SEED_V1"), F_ = 3, I_ = 1, L_ = 5, R_ = 8979, z_ = 4096, B_ = 79, V_ = 1568, H_ = 24, U_ = 5785;
function W_(e, t) {
	try {
		return e instanceof Uint8Array ? new Uint8Array(e) : _(e);
	} catch {
		throw Error(`${t} must be hexadecimal data`);
	}
}
function G_(e) {
	let t = /* @__PURE__ */ new Uint8Array(8);
	return new DataView(t.buffer).setBigUint64(0, BigInt(e), !1), t;
}
function K_(...e) {
	let t = e.reduce((e, t) => e + t.length, 0), n = new Uint8Array(t), r = 0;
	for (let t of e) n.set(t, r), r += t.length;
	return n;
}
function q_(e) {
	return K_(G_(e.length), e);
}
function J_(e, t, n, r) {
	return bn(K_(j_, G_(e), q_(new TextEncoder().encode(t)), q_(new TextEncoder().encode(n)), q_(r)));
}
function Y_(e, t, n, r) {
	return K_(A_, new Uint8Array([
		F_,
		I_,
		L_
	]), G_(e), J_(e, t, n, r));
}
function X_(e, t, n, r) {
	return tg(bn, e, bn(K_(M_, t, n, r)), N_, 32);
}
function Z_(e, t, n, r, i) {
	if (e.length !== U_) throw Error("invalid Antartical phone envelope size");
	let a = Y_(t, n, r, i);
	for (let t = 0; t < B_; t++) if (e[t] !== a[t]) throw Error("invalid Antartical phone envelope context");
}
function Q_(e) {
	let t = W_(e, "device private key");
	if (t.length !== 32) throw Error("device private key must be 32 bytes");
	return bn(K_(P_, t));
}
function $_(e) {
	let t = W_(e, "phone encryption seed");
	if (t.length !== 64) throw Error("phone encryption seed must be 64 bytes");
	return k_.keygen(t).publicKey;
}
function ev(e, t = R_, n, r, i, a) {
	let o = W_(e, "recipient ML-KEM public key");
	if (o.length !== V_) throw Error("recipient ML-KEM public key must be 1568 bytes");
	let s = W_(i, "phone nonce");
	if (s.length === 0) throw Error("phone nonce is required");
	let c = typeof a == "string" ? new TextEncoder().encode(a) : W_(a, "plaintext");
	if (c.length > z_) throw Error("phone plaintext exceeds 4096 bytes");
	let l = Y_(t, n, r, s), { cipherText: u, sharedSecret: d } = k_.encapsulate(o), f = X_(d, o, l, u), p = /* @__PURE__ */ new Uint8Array(4098);
	new DataView(p.buffer).setUint16(0, c.length, !1), p.set(c, 2);
	let m = crypto.getRandomValues(new Uint8Array(H_)), h = K_(l, u, m);
	return S(K_(h, Xh(f, m, h).encrypt(p)));
}
function tv(e, t, n = R_, r, i, a) {
	let o = W_(e, "phone encryption seed"), s = W_(t, "phone envelope"), c = W_(a, "phone nonce");
	if (o.length !== 64) throw Error("phone encryption seed must be 64 bytes");
	Z_(s, n, r, i, c);
	let l = s.slice(0, B_), u = s.slice(B_, 1647), d = s.slice(1647, 1671), f = s.slice(0, 1671), p = s.slice(1671), m = k_.keygen(o).secretKey, h = k_.decapsulate(u, m), g = k_.keygen(o).publicKey, _ = X_(h, g, l, u), v;
	try {
		v = Xh(_, d, f).decrypt(p);
	} catch {
		throw Error("invalid Antartical phone envelope authentication");
	}
	let y = new DataView(v.buffer, v.byteOffset, v.byteLength).getUint16(0, !1);
	if (v.length !== 4098 || y > z_) throw Error("invalid Antartical phone envelope payload");
	for (let e = 2 + y; e < v.length; e++) if (v[e] !== 0) throw Error("invalid Antartical phone envelope padding");
	return v.slice(2, 2 + y);
}
//#endregion
//#region phone.js
var nv = H("TKMPHONE_PQ_V1");
function rv(e, t) {
	let n = _(t);
	if (n.length !== 32) throw Error("Phone signing hash must be 32 bytes.");
	let r = Jm.keygen(e);
	try {
		let e = Jm.sign(_(C([nv, n])), r.secretKey);
		return S(C([
			nv,
			r.publicKey,
			e
		]));
	} finally {
		r.secretKey.fill(0);
	}
}
async function iv(e, t, n) {
	let r = await sh(Zm(e), t);
	try {
		return rv(r, n);
	} finally {
		r.fill(0);
	}
}
async function av(e, t) {
	let n = Zm(e), r = await sh(n, t);
	try {
		return "0x" + n.publicKey.replace(/^0x/, "");
	} finally {
		r.fill(0);
	}
}
async function ov(e, t) {
	let n = await sh(Zm(e), t);
	try {
		return S($_(Q_(n)));
	} finally {
		n.fill(0);
	}
}
async function sv(e, t, n, r, i, a) {
	return ev(e, t, n, r, i, a);
}
async function cv(e, t, n, r, i, a, o) {
	let s = await sh(Zm(e), t);
	try {
		return S(tv(Q_(s), o, n, r, i, a));
	} finally {
		s.fill(0);
	}
}
//#endregion
//#region vendor/email-crypto.js
var lv = new TextEncoder(), uv = new TextDecoder(), dv = lv.encode("TKM_EMAILVM_X25519_SALT_V1"), fv = lv.encode("TKM_EMAILVM_X25519_KEY_V1"), pv = lv.encode("TKM_EMAILVM_XCHACHA20POLY1305_V1");
function mv(...e) {
	let t = e.reduce((e, t) => e + t.length, 0), n = new Uint8Array(t), r = 0;
	for (let t of e) n.set(t, r), r += t.length;
	return n;
}
function hv(e, t) {
	return lv.encode(`${String(e).toLowerCase()}\n${String(t).toLowerCase()}`);
}
function gv(e) {
	let t = tg(Yt, e, dv, fv, 32);
	return {
		privateKey: t,
		publicKey: kh.getPublicKey(t)
	};
}
function _v(e) {
	if (!globalThis.crypto?.getRandomValues) throw Error("secure browser randomness is unavailable; use HTTPS");
	return globalThis.crypto.getRandomValues(e);
}
function vv(e, t, n, r) {
	let i = kh.getSharedSecret(e, t);
	try {
		return tg(Yt, i, hv(n, r), pv, 32);
	} finally {
		i.fill(0);
	}
}
function yv(e, t, n, r, i, a = _v) {
	let o = a(/* @__PURE__ */ new Uint8Array(24)), s = mv(pv, hv(n, r)), c = vv(e, t, n, r);
	try {
		return {
			ciphertext: Xh(c, o, s).encrypt(lv.encode(i)),
			nonce: o
		};
	} finally {
		c.fill(0);
	}
}
function bv(e, t, n, r, i, a) {
	let o = mv(pv, hv(n, r)), s = vv(e, t, n, r);
	try {
		return uv.decode(Xh(s, a, o).decrypt(i));
	} finally {
		s.fill(0);
	}
}
function xv(e) {
	e?.privateKey?.fill(0);
}
//#endregion
//#region vendor/email-registry.js
var Sv = "TKM_EMAILVM_REGISTRY_V1";
function Cv(e, t) {
	if (e !== "domain" && e !== "mailbox") throw Error("invalid EmailVM registry kind");
	if (!t || t !== t.toLowerCase()) throw Error("EmailVM registry names must be canonical lowercase values");
	return W(H(`${Sv}\0${e}\0${t}`));
}
//#endregion
//#region mail.js
var wv = 3000000n, Tv = (1n << 64n) - 1n, Ev = (e) => String(e || "").toLowerCase(), Dv = (e) => String(e || "").trim().toLowerCase();
function Ov(e, t, n, r, i) {
	let a = BigInt(t), o = BigInt(r), s = 0;
	for (let t of e) {
		if (!a) break;
		if (t.status !== "available" || Number(t.version) !== 2 || BigInt(t.assetId) !== 1n) continue;
		let e = o < n ? n - o : 0n;
		if (e && !i) break;
		let r = BigInt(t.noteValueWei) - e;
		r <= 0n || (a -= a < r ? a : r, o -= n - e, s++);
	}
	for (; a > 0n;) {
		let e = a < Tv ? a : Tv;
		if (o < e + 2n * n) throw Error("Insufficient total funds for the complete Mail payment and network gas.");
		o -= e + 2n * n, a -= e, s += 2;
	}
	return {
		transactions: s,
		maxGas: BigInt(s) * n
	};
}
function kv(e, t) {
	let n = _(e.applicationData), r = H("TKMEMAILVM1");
	if (n.length > 12288 || S(n.slice(0, r.length)) !== S(r)) throw Error("Invalid mail action encoding.");
	let i = JSON.parse(me(n.slice(r.length)));
	if (i.v !== 3) throw Error("Unsupported mail action version.");
	for (let [e, n] of Object.entries(t)) if (Ev(i[e]) !== Ev(n)) throw Error("Node changed the mail action: " + e);
	return i;
}
function Av(e, { nonce: t, gasPrice: n, note: r, amount: i, sponsor: a, recipient: o, address: s, applicationData: c }) {
	let l = e.transaction;
	if (jg(l, {
		chainId: 8979,
		value: 0n
	}), BigInt(l.nonce) !== BigInt(t) || BigInt(l.gas) !== wv || BigInt(l.gasFeeCap) !== n || BigInt(l.gasTipCap) !== n || !Array.isArray(l.accessList) || l.accessList.length) throw Error("Proof builder changed mail transaction fees or nonce.");
	if (bg(l, e) !== a) throw Error("Proof builder changed mail gas sponsorship.");
	let u = BigInt(r.noteValueWei) - i - a;
	if (u < 0n) throw Error("Insufficient note value for mail payment and gas.");
	yg(l, e, {
		recipient: o,
		valueWei: i,
		expectedOutputs: u ? [{
			index: 0,
			recipient: s,
			assetId: 1n,
			valueWei: u
		}] : []
	}), xg(l, c);
	let d = Je(_(l.data).slice(10));
	if (d[1].length !== 1 || Ev(d[1][0][0]) !== Ev(r.nullifier) || Ev(e.spentNullifier) !== Ev(r.nullifier)) throw Error("Proof builder changed the mail input note.");
	return l;
}
async function jv({ keystore: e, password: t, rpc: n, proverURL: r, proverFetch: i, operation: a, params: o = {}, onReview: s, onProgress: c = () => {}, onSubmitted: l = () => {} }) {
	let u = Rg(), d, f, p, m = [];
	try {
		let u = Zm(e);
		d = await sh(u, t);
		let h = K("0x" + u.address.replace(/^0x/, ""));
		if (f = wg(d, h, 8979), p = gv(d), BigInt(await n("eth_chainId", [])) !== 8979n) throw Error("Mail requires TKM chain 8979.");
		let g = new Ig({ send: n }), v = new Ag(r, "", i), y = {
			lastScannedBlock: -1,
			notes: []
		}, b = S(p.publicKey), x = async (e) => {
			let t = await n("tkmdomain_mailbox", [e]);
			if (Ev(t.owner) !== Ev(h)) throw Error("Selected PQ account does not own " + e);
			return t;
		}, C = async (e) => {
			let t = uh(e, d), r = W(t.rawTransaction);
			if (m.push(r), await l(r), Ev(await n("eth_sendRawTransaction", [t.rawTransaction])) !== Ev(r)) throw Error("Node returned an unexpected transaction hash.");
			for (let e = 0; e < 150; e++) {
				c("Waiting for confirmation: " + r);
				let e = await n("eth_getTransactionReceipt", [r]);
				if (e) {
					if (BigInt(e.status) !== 1n) throw Error("Mail transaction reverted: " + r);
					return r;
				}
				await new Promise((e) => setTimeout(e, 2e3));
			}
			throw Error("Confirmation timed out. Check the recorded transaction before retrying.");
		}, w = async () => {
			let e = await n("eth_getTransactionCount", [h, "latest"]);
			if (BigInt(await n("eth_getTransactionCount", [h, "pending"])) !== BigInt(e)) throw Error("The account has a pending transaction. Wait before retrying.");
			return e;
		}, T = async (e, t, r, i) => {
			if (!await g.shieldedV2Active()) throw Error("Shield2 must be active.");
			let a = await v.health();
			if (!a.ok || !a.hasProvingKeyV2 || !a.withdrawalBuildReady) throw Error("Local Mail proof builder is not ready.");
			let o = BigInt(await n("eth_gasPrice", [])), l = wv * o;
			await Pg(g, f, y, (e, t) => c(`Scanning notes: ${e} / ${t}`));
			let u = Ov(y.notes, r, l, BigInt(await n("eth_getBalance", [h, "latest"])), await g.shieldedGasSponsorActive());
			if (!s || !await s({
				label: i,
				recipient: t,
				amount: nt(r),
				gasPerTransaction: nt(l),
				maxGas: nt(u.maxGas),
				transactions: u.transactions
			})) throw Error("Operation cancelled.");
			let d = r;
			for (; d > 0n;) {
				await Pg(g, f, y, (e, t) => c(`Scanning notes: ${e} / ${t}`));
				let r = BigInt(await n("eth_getBalance", [h, "latest"])), a = r < l ? l - r : 0n;
				if (a && !await g.shieldedGasSponsorActive()) throw Error("Public balance cannot cover network gas.");
				let s = y.notes.find((e) => e.status === "available" && Number(e.version) === 2 && BigInt(e.assetId) === 1n && BigInt(e.noteValueWei) > a);
				if (!s) {
					let e = d < Tv ? d : Tv;
					if (r < e + 2n * l) throw Error("Insufficient public balance for self-shielding and Mail gas.");
					let t = await w();
					c("Creating a private note for Mail…");
					let n = Ug(await v.buildDeposit({
						requestId: "mail-fund-" + S($n(16)),
						from: h,
						to: h,
						amountWei: e.toString(),
						assetId: "1",
						ownerSecret: BigInt(h).toString(),
						recipientViewKey: S(f.viewPublicKey),
						nonce: t,
						gasPriceWei: o.toString()
					}), {
						nonce: t,
						gasPrice: o,
						part: {
							note: null,
							value: e,
							sponsor: 0n
						},
						address: h,
						recipient: f
					});
					await C(n);
					continue;
				}
				let u = BigInt(s.noteValueWei) - a, p = d < u ? d : u;
				if ((await g.privacyNullifierStatus(s.nullifier))?.spent) {
					s.status = "spent";
					continue;
				}
				let m = await g.privacyCommitmentPath(s.commitment);
				if (!m.found) throw Error("Mail input note is no longer confirmed.");
				s = {
					...s,
					merkleRoot: m.root,
					merklePath: m.merklePath.map(String),
					merklePathIndex: m.merklePathIndex.map((e) => BigInt(e).toString())
				};
				let _ = await w();
				c("Building " + i + " proof…");
				let b = Av(await v.buildWithdrawal({
					requestId: "mail-" + S($n(16)),
					applicationData: e.applicationData,
					from: h,
					to: t,
					amountWei: p.toString(),
					changeViewKey: S(f.viewPublicKey),
					nonce: _,
					gasPriceWei: o.toString(),
					note: s
				}), {
					nonce: _,
					gasPrice: o,
					note: s,
					amount: p,
					sponsor: a,
					recipient: t,
					address: h,
					applicationData: e.applicationData
				});
				await C(b), d -= p;
				let x = y.notes.find((e) => e.nullifier === s.nullifier);
				x && (x.status = "spent");
			}
		}, E = async (e) => {
			let t = await x(e);
			if (Ev(t.encryptionKey) === Ev(b)) return;
			if (t.encryptionKey && t.encryptionKey !== "0x") throw Error("This mailbox uses a different mail key. Refusing to replace it and lose access to existing mail.");
			let r = await n("emailvm_publishKey", [e, b]);
			if (kv(r, {
				kind: "key",
				mailbox: e,
				key: b.slice(2)
			}), await T(r, h, 1n, "Publish encryption key for " + e), Ev((await n("emailvm_key", [e])).publicKey) !== Ev(b)) throw Error("Encryption key is not yet indexed. Check publication before sending.");
		};
		if (a === "buy") {
			let e = Dv(o.username), t = Dv(o.domain).replace(/^@/, ""), r = e + "@" + t, i = await n("tkmdomain_domain", [t]), a = K(i.payoutAddress && i.payoutAddress !== "0x0000000000000000000000000000000000000000" ? i.payoutAddress : i.operator), s = BigInt(await n("tkmdomain_subscriberUnitPrice", [])), c = await n("tkmdomain_buy", [e, t]), l = Cv("mailbox", r);
			if (kv(c, {
				kind: "buy",
				username: e,
				domain: t,
				registryHash: l
			}), Ev(c.registryHash) !== Ev(l) || Ev(c.withdrawalRecipient) !== Ev(a) || BigInt(c.totalWithdrawalAmountWei) !== s) throw Error("Mailbox payment differs from the domain price or payout address.");
			let u = (await n("tkmdomain_pending", []) || []).find((n) => n.kind === "buy" && n.domain === t && n.username === e && Ev(n.payer) === Ev(h) && Ev(n.recipient) === Ev(a) && BigInt(n.required) === s), d = s - BigInt(u?.paid || 0);
			if (d <= 0n) throw Error("Mailbox payment is awaiting indexing. Check registration before retrying.");
			await T(c, a, d, "Register " + r), await x(r), await E(r);
		} else if (a === "publish") await E(Dv(o.mailbox));
		else if (a === "send") {
			let e = Dv(o.from), t = Dv(o.to), r = String(o.subject || "").trim() + "\n\n" + String(o.body || "");
			if (!r.trim() || H(r).length > 4e3) throw Error("Write a message of at most 4,000 UTF-8 bytes including subject.");
			if (Ev((await x(e)).encryptionKey) !== Ev(b)) throw Error("Publish this wallet’s Mail encryption key before sending.");
			let i = await n("emailvm_key", [t]), a = yv(p.privateKey, _(i.publicKey), e, t, r), s = S(a.ciphertext), c = S(a.nonce), l = await n("emailvm_send", [
				e,
				t,
				s,
				c
			]);
			kv(l, {
				kind: "message",
				from: e,
				to: t,
				ciphertext: s.slice(2),
				nonce: c.slice(2)
			}), await T(l, h, 1n, "Send encrypted mail to " + t);
		} else if (a === "decrypt") {
			let e = Dv(o.mailbox), t = o.message;
			if (Ev((await x(e)).encryptionKey) !== Ev(b)) throw Error("This wallet does not hold the published Mail key.");
			let r = Dv(t.from), i = Dv(t.to);
			if (r !== e && i !== e) throw Error("Message does not belong to this mailbox.");
			let a = await n("emailvm_key", [r === e ? i : r]);
			return bv(p.privateKey, _(a.publicKey), r, i, _(t.ciphertext), _(t.nonce));
		} else throw Error("Unsupported Mail operation.");
		return m;
	} catch (e) {
		throw m.length && (e.message += " Attempted transaction hashes: " + m.join(", ") + ". Check before retrying."), e;
	} finally {
		d?.fill(0), Tg(f), xv(p), u();
	}
}
//#endregion
//#region shield3-migrate.js
var Mv = 3000000n;
function Nv(e, { address: t, note: n, nonce: r, gasPrice: i }) {
	let a = e.transaction;
	if (jg(a, {
		chainId: 8979,
		value: 0n
	}), yg(a, e, {
		recipient: t,
		valueWei: BigInt(n.noteValueWei)
	}), BigInt(a.nonce) !== BigInt(r) || BigInt(a.gas) !== Mv || BigInt(a.gasFeeCap) !== i || BigInt(a.gasTipCap) !== i || a.accessList?.length || bg(a, e) !== 0n) throw Error("Migration proof changed nonce, fees or gas sponsorship.");
	let o = Je(_(a.data).slice(10));
	if (o[1]?.length !== 1 || S(o[1][0][0]).toLowerCase() !== n.nullifier.toLowerCase() || S(o[1][0][1]).toLowerCase() !== n.merkleRoot.toLowerCase() || o[8]?.length !== 4) throw Error("Migration proof changed the input or omitted empty-output openings.");
	for (let e = 0; e < 4; e++) {
		let n = BigInt(o[8][e]);
		if (n <= 0n || n >= 21888242871839275222246405745257275088548364400416034343698204186575808495617n) throw Error("Migration proof returned invalid randomness.");
		if (te(gg([
			2001n,
			e === 0 ? BigInt(t) : 0n,
			1n,
			0n,
			n
		]), 32).toLowerCase() !== S(o[2][e][0]).toLowerCase()) throw Error("Migration proof creates private change.");
	}
	return a;
}
async function Pv(e) {
	let t = Rg(), n, r, i;
	try {
		let t = Zm(e.keystore);
		n = await sh(t, e.password);
		let a = K("0x" + t.address.replace(/^0x/, ""));
		r = wg(n, a, 8979);
		let o = new Oe(e.rpcURL);
		if (o.setHeader("X-GUI-Token", e.rpcToken), i = new Vd(o), BigInt(await i.send("eth_chainId", [])) !== 8979n || !(await i.send("tkmprivacy_shieldedV3Status", [])).active) throw Error("Migration requires Antartical on TKM mainnet.");
		let s = new Ig(i), c = {
			lastScannedBlock: -1,
			notes: []
		};
		e.onProgress?.("Scanning legacy notes…"), await Pg(s, r, c, () => {});
		let l = c.notes.find((e) => e.status === "available" && Number(e.version) === 2 && BigInt(e.assetId) === 1n);
		if (!l) throw Error("No confirmed Shield2 note remains to migrate.");
		let u = await s.getTransactionCount(a, "latest");
		if (BigInt(await s.getTransactionCount(a, "pending")) !== BigInt(u)) throw Error("Wait for your pending transaction to confirm.");
		let d = BigInt(await i.send("eth_gasPrice", []));
		if (BigInt(await i.send("eth_getBalance", [a, "latest"])) + BigInt(l.noteValueWei) < Mv * d) throw Error("This note and public balance cannot cover migration gas.");
		if ((await s.privacyNullifierStatus(l.nullifier))?.spent) throw Error("Legacy note is already spent. Rescan.");
		let f = await s.privacyCommitmentPath(l.commitment);
		if (!f.found) throw Error("Legacy note is no longer confirmed.");
		let p = {
			...l,
			merkleRoot: f.root,
			merklePath: f.merklePath.map(String),
			merklePathIndex: f.merklePathIndex.map((e) => BigInt(e).toString())
		}, m = new Ag(new URL("/prover", globalThis.location.href).href, "", (t, n) => fetch(t, {
			...n,
			headers: {
				...n.headers,
				"X-GUI-Token": e.rpcToken
			}
		}));
		e.onProgress?.("Building the full-note migration proof…");
		let h = uh(Nv(await m.buildWithdrawal({
			requestId: S($n(16)),
			from: a,
			to: a,
			amountWei: p.noteValueWei,
			note: p,
			changeViewKey: S(r.viewPublicKey),
			nonce: u,
			gasPriceWei: d.toString()
		}), {
			address: a,
			note: p,
			nonce: u,
			gasPrice: d
		}), n), g = W(h.rawTransaction);
		e.onSubmitted?.(g, nt(BigInt(p.noteValueWei)));
		try {
			await s.sendRawTransaction(h.rawTransaction);
		} catch (e) {
			throw Error("Migration submission status uncertain: " + g + ". Check Activity before retrying. " + e.message);
		}
		return g;
	} finally {
		n?.fill(0), Tg(r), i?.destroy(), t();
	}
}
//#endregion
//#region engine.js
async function Fv(e) {
	return (await Jg(e)).active ? i_(e) : Wg(e);
}
function Iv() {
	return cf.fromEntropy($n(32)).phrase;
}
function Lv(e) {
	let t = String(e).normalize("NFKD").trim().toLowerCase().split(/\s+/).join(" ");
	if (t.split(" ").length !== 24) throw Error("Enter all 24 recovery words.");
	let n = cf.fromPhrase(t);
	if (_(n.entropy).length !== 32) throw Error("Expected a 24-word TKM PQ recovery phrase.");
	return n.entropy;
}
async function Rv(e, t) {
	let n = await sh(Zm(e), t);
	try {
		return cf.fromEntropy(n).phrase;
	} finally {
		n.fill(0);
	}
}
function zv(e) {
	return JSON.parse(me(e));
}
function Bv(e) {
	return Dg(String(e).trim(), 8979);
}
async function Vv({ keystore: e, password: t, rpcURL: n, rpcToken: r, onProgress: i }) {
	if ((await Jg({
		rpcURL: n,
		rpcToken: r
	})).active) return nt((await r_({
		keystore: e,
		password: t,
		rpcToken: r
	})).balanceWei);
	let a = Zm(e), o = await sh(a, t), s, c;
	try {
		s = wg(o, K("0x" + a.address.replace(/^0x/, "")), 8979);
		let e = new Oe(n);
		if (e.setHeader("X-GUI-Token", r), c = new Vd(e), BigInt(await c.send("eth_chainId", [])) !== 8979n) throw Error("This wallet requires TKM mainnet.");
		let t = {
			lastScannedBlock: -1,
			notes: []
		};
		return await Pg(new Ig(c), s, t, i), nt(Fg(t.notes));
	} finally {
		o.fill(0), Tg(s), c?.destroy();
	}
}
//#endregion
export { Dg as decodeShieldedPaymentCode, zv as keyfileFromHex, Pv as migrateShield2Note, Iv as newRecoveryPhrase, cv as phoneDecryptV2, sv as phoneEncryptV2, ov as phoneEncryptionPublicKey, av as phonePublicKey, Rv as recoveryPhraseForKeyfile, jv as runMailOperation, Vv as scanWallet, Lv as seedFromPhrase, Fv as sendTKM, u_ as shield3Disclosure, n_ as shield3FetchRelayOffer, a_ as shield3Funds, Qg as shield3Identity, o_ as shield3RegisterStamp, c_ as shield3Relay, l_ as shield3ReviewRelayOffer, r_ as shield3Scan, s_ as shield3StampSponsorship, Jg as shield3Status, $g as shield3ViewKeys, e_ as shield3ViewScan, t_ as shield3ViewStamp, iv as signPhoneDigest, Bv as validateRecipient, Kg as validateShield3Amount, qg as validateShield3Payments, Yg as validateShield3Recipient };
