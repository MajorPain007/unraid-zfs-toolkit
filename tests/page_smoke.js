// Executes the page's JavaScript against a stubbed DOM and fails if it throws.
//
// Syntax checks and name resolution both pass on a script that dies on the
// first statement. That is what happened with a top-level var read before its
// assignment: node --check was happy, every call resolved, and the page was
// dead - no tabs, no layout, no data. Running it is the only check that sees
// that class of fault.
//
// The stub answers everything and throws nothing, so a failure here is the
// page's, not the stub's.

const fs = require('fs');
const vm = require('vm');
const path = require('path');

const pagePath = process.argv[2] || path.join(__dirname, '..', 'src', 'ZFSToolkitPage.php');
const source = fs.readFileSync(pagePath, 'utf8');

// Take the script bodies out of the PHP page. Anything PHP prints into them is
// left as-is; the point is the JavaScript around it.
const blocks = [...source.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g)]
  .map(m => m[1]);
if (blocks.length === 0) {
  console.error('no inline script found in ' + pagePath);
  process.exit(1);
}
const js = blocks.join('\n');

function makeElement() {
  const el = {
    style: new Proxy({}, {get: () => '', set: () => true}),
    dataset: {},
    classList: {add() {}, remove() {}, toggle() {}, contains: () => false},
    // Every form field reports the same value, and which value decides which
    // branches run. One pass with an empty value leaves most switch statements
    // on their default and proves almost nothing, so run.sh runs this several
    // times with different values.
    value: process.env.ZDC_STUB_VALUE || '',
    textContent: '', innerHTML: '', checked: false,
    offsetHeight: 100, offsetWidth: 100, scrollHeight: 100, scrollTop: 0,
    disabled: false, title: '', className: '', id: '', hidden: false,
    getAttribute: () => null,
    setAttribute() {}, removeAttribute() {},
    appendChild(c) { return c; },
    insertRow: () => makeElement(),
    insertCell: () => makeElement(),
    addEventListener() {}, removeEventListener() {},
    querySelector: () => makeElement(),
    querySelectorAll: () => [],
    closest: () => makeElement(),
    focus() {}, blur() {}, click() {}, remove() {},
    getBoundingClientRect: () => ({top: 0, left: 0, width: 100, height: 100}),
    rows: {length: 0},
    options: [],
    parentNode: null,
  };
  el.parentNode = el;
  return el;
}

const el = makeElement();

const thenable = {
  then(fn) { return thenable; },
  catch(fn) { return thenable; },
  finally(fn) { return thenable; },
};

const jq = new Proxy(function () { return jq; }, {
  get(_t, k) {
    if (k === Symbol.toPrimitive || k === 'toString') return () => '';
    if (k === 'length') return 0;
    return () => jq;
  },
  apply() { return jq; },
});

const context = {
  console: {log() {}, warn() {}, error() {}, info() {}},
  setTimeout: () => 0,
  setInterval: () => 0,
  clearTimeout() {}, clearInterval() {},
  requestAnimationFrame: () => 0,
  fetch: () => thenable,
  Promise, JSON, Math, Date, RegExp, Error, Array, Object, String, Number, Boolean,
  parseInt, parseFloat, isNaN, encodeURIComponent, decodeURIComponent,
  URLSearchParams: function () { this.append = () => {}; this.toString = () => ''; },
  FormData: function () { this.append = () => {}; },
  ResizeObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
  MutationObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
  localStorage: {getItem: () => null, setItem() {}, removeItem() {}},
  getComputedStyle: () => ({getPropertyValue: () => '#111111', backgroundColor: 'rgb(17, 17, 17)'}),
  alert() {}, confirm: () => true, prompt: () => null,
  csrf_token: 'stub-token',
  jQuery: jq,
  $: jq,
  navigator: {userAgent: 'stub'},
  addEventListener() {}, removeEventListener() {}, dispatchEvent: () => true,
  matchMedia: () => ({matches: false, addEventListener() {}, addListener() {}}),
  scrollTo() {}, innerWidth: 1400, innerHeight: 900, devicePixelRatio: 1,
  Event: function () {}, CustomEvent: function () {},
  location: {href: 'http://stub/', search: '', pathname: '/'},
  document: {
    getElementById: () => el,
    querySelector: () => el,
    querySelectorAll: () => [],
    createElement: () => makeElement(),
    createTextNode: () => makeElement(),
    addEventListener() {},
    body: el,
    documentElement: el,
    readyState: 'complete',
    cookie: '',
  },
};
context.window = context;
context.globalThis = context;

try {
  vm.createContext(context);
  vm.runInContext(js, context, {filename: 'ZFSToolkitPage.js', timeout: 10000});
} catch (e) {
  console.error(e && e.stack ? e.stack.split('\n').slice(0, 4).join('\n') : String(e));
  process.exit(1);
}
process.exit(0);
