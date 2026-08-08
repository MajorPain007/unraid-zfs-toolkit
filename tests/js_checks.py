#!/usr/bin/env python3
"""Static checks for the inline JavaScript of the settings page.

php -l and node --check both accept a call to a function that does not exist,
so an edit that drops a helper produces a page that parses fine and then dies
at runtime with "X is not defined". This finds those before shipping.

Checks:
  1. every function called from the page's own code is defined
  2. every function referenced from an on* HTML attribute is defined
  3. every getElementById(...) id used by the JS exists in the markup

Exit code 0 = clean, 1 = problems found.
"""
import re
import sys
from pathlib import Path

BUILTINS = {
    # globals and constructors
    'Array', 'Boolean', 'Date', 'Error', 'JSON', 'Math', 'Number', 'Object',
    'Option', 'Promise', 'RegExp', 'String', 'URLSearchParams', 'Set', 'Map',
    'FormData', 'Blob', 'Image', 'Event',
    # functions
    'alert', 'confirm', 'prompt', 'fetch', 'parseInt', 'parseFloat', 'isNaN',
    'isFinite', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval',
    'encodeURIComponent', 'decodeURIComponent', 'encodeURI', 'decodeURI',
    'requestAnimationFrame', 'structuredClone', 'queueMicrotask',
    # control flow keywords that the naive regex would otherwise pick up
    'if', 'for', 'while', 'switch', 'catch', 'return', 'typeof', 'function',
    'new', 'do', 'else', 'delete', 'void', 'in', 'of', 'instanceof', 'await',
    'yield', 'throw', 'case', 'const', 'let', 'var',
    # provided by the Unraid page shell
    'csrf_token', 'jQuery', '$', 'swal', 'autov', 'addRemoveTab',
}


def extract_script(text):
    m = re.search(r'<script>(.*?)</script>', text, re.S)
    return m.group(1) if m else ''


def defined_names(js):
    names = set(re.findall(r'\bfunction\s+([A-Za-z_$][\w$]*)\s*\(', js))
    names |= set(re.findall(r'\b(?:var|let|const)\s+([A-Za-z_$][\w$]*)\s*=\s*function', js))
    names |= set(re.findall(r'\b(?:var|let|const)\s+([A-Za-z_$][\w$]*)\s*=\s*\(?[\w\s,]*\)?\s*=>', js))
    return names


def strip_literals(js):
    """Remove string literals and comments so their contents are not scanned."""
    js = re.sub(r'/\*.*?\*/', ' ', js, flags=re.S)
    js = re.sub(r'(?m)//.*$', ' ', js)
    js = re.sub(r"'(?:[^'\\\n]|\\.)*'", "''", js)
    js = re.sub(r'"(?:[^"\\\n]|\\.)*"', '""', js)
    return js


def called_names(js_code):
    out = set()
    for m in re.finditer(r'(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(', js_code):
        out.add(m.group(1))
    return out


def handler_names(text):
    """Function names referenced from on* attributes, including inside JS strings
    that build markup (that is how the file renders its tables)."""
    out = set()
    for m in re.finditer(r'\bon[a-z]+\s*=\s*(["\'])(.*?)\1', text, re.S):
        for c in re.finditer(r'(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(', m.group(2)):
            out.add(c.group(1))
    return out


def element_ids(text):
    ids = set(re.findall(r'\bid\s*=\s*"([A-Za-z0-9_-]+)"', text))
    ids |= set(re.findall(r"\bid\s*=\s*'([A-Za-z0-9_-]+)'", text))
    # ids assembled inside JS string concatenation, e.g. 'id="row-' + i + '"'
    ids |= set(re.findall(r'\bid="([A-Za-z0-9_-]+)\'', text))
    return ids


def main():
    path = Path(sys.argv[1] if len(sys.argv) > 1 else 'src/ZFSDatasetConverterPage.php')
    text = path.read_text()
    js = extract_script(text)
    if not js:
        print('  no <script> block found')
        return 1

    problems = []

    defined = defined_names(js)
    code = strip_literals(js)

    undefined_calls = sorted(called_names(code) - defined - BUILTINS)
    for name in undefined_calls:
        problems.append(f'called but never defined: {name}()')

    undefined_handlers = sorted(handler_names(text) - defined - BUILTINS)
    for name in undefined_handlers:
        problems.append(f'referenced from an on* attribute but never defined: {name}()')

    outside = re.sub(r'<script>.*?</script>', ' ', text, flags=re.S)
    for m in re.finditer(r'\\u[0-9a-fA-F]{4}', outside):
        ctx = outside[max(0, m.start() - 40):m.end() + 10].replace('\n', ' ')
        problems.append(f'literal escape {m.group(0)} in HTML text (renders verbatim): ...{ctx.strip()}')

    for m in re.finditer(r'var\(--[A-Za-z0-9-]+\)[0-9a-fA-F]{2}\b', text):
        problems.append(f'mangled colour {m.group(0)!r}: an 8-digit hex lost its '
                        f'alpha pair when the variable was substituted; use rgba()')

    ids_in_markup = element_ids(text)
    used_ids = set(re.findall(r"getElementById\(\s*'([A-Za-z0-9_-]+)'\s*\)", js))
    used_ids |= set(re.findall(r'getElementById\(\s*"([A-Za-z0-9_-]+)"\s*\)', js))
    for i in sorted(used_ids - ids_in_markup):
        problems.append(f'getElementById("{i}") but no element with that id')

    if problems:
        for p in problems:
            print(f'  {p}')
        return 1

    print(f'  {len(defined)} functions defined, all calls and handlers resolve, '
          f'{len(used_ids)} element ids resolve')
    return 0


if __name__ == '__main__':
    sys.exit(main())
