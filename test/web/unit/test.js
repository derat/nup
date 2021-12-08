// Copyright 2021 Daniel Erat.
// All rights reserved.

// TODO: Decide if there's a cleaner way to do this. Right now, this stuff gets
// shoved into globals so it can be accessed by exported functions in an
// approximation of the style of other JS test frameworks.
const initResult = newResult('init'); // errors outside of tests
const allSuites = []; // Suite objects registered via suite()
let curSuite = null; // Suite currently being added via suite()
let results = []; // test results from current run
let curResult = null; // in-progress test's result
let lastDone = null; // promise resolve func from last async test

// Suite contains a collection of tests.
class Suite {
  constructor(name) {
    this.name = name;
    this.tests = []; // Test objects
    this.beforeAlls = [];
    this.afterAlls = [];
    this.beforeEaches = [];
    this.afterEaches = [];
  }

  // Sequentially run all tests in the suite.
  // If |testName| is supplied, only the test with that name is run.
  async runTests(testName) {
    const tests = this.tests.filter(
      (t) => testName === undefined || t.name === testName
    );
    if (!tests.length) throw new Error(`No test ${this.name}.${testName}`);

    try {
      this.beforeAlls.forEach((f) => f());
      for (const t of tests) {
        const fullName = `${this.name}.${t.name}`;
        console.log('-'.repeat(80));
        console.log(`Starting ${fullName}`);
        curResult = newResult(fullName);
        try {
          this.beforeEaches.forEach((f) => f());
          await t.run();
          this.afterEaches.forEach((f) => f());
        } finally {
          console.log(
            `Finished ${fullName} with ${curResult.errors.length} error(s)`
          );
          results.push(curResult);
          curResult = null;
        }
      }
    } finally {
      this.afterAlls.forEach((f) => f());
    }
  }
}

// Test contains an individual test.
class Test {
  constructor(name, f) {
    this.name = name;
    this.func = f;
  }

  // Runs the test to completion. This could probably be simplified, but I'm
  // terrible at promises and async functions. :-/
  async run() {
    // First, create a promise to get a 'done' callback to pass to tests that
    // want one.
    await new Promise((done) => {
      try {
        // Save the 'done' callback to a global variable so it can be called by
        // the 'error' and 'unhandledrejection' handlers if an error happens in
        // e.g. a timeout, which runs in the window execution context.
        lastDone = done;

        Promise.resolve(this.func(done))
          .catch((reason) => {
            // This handles exceptions thrown directly from async tests.
            // It also handles rejected promises from async tests.
            if (reason instanceof Error) handleException(reason);
            else handleRejection(reason);
            done();
          })
          .then(() => {
            // If the test doesn't take any args, run the 'done' callback for it.
            if (!this.func.length) done();
          });
      } catch (err) {
        // This handles exceptions thrown directly from synchronous tests.
        handleException(err);
        done();
      }
    });

    lastDone = null;
  }
}

// Adds a test suite named |name|.
// |f| is is executed immediately; it should call test() to define the
// suite's tests.
export function suite(name, f) {
  if (curSuite) throw new Error(`Already adding suite ${curSuite.name}`);

  const s = new Suite(name);
  try {
    curSuite = s;
    f();
    if (!s.tests.length) throw new Error('No tests defined');
    allSuites.push(s);
  } catch (err) {
    handleException(err);
  } finally {
    curSuite = null;
  }
}

// Adds |f| to run before all tests in the suite.
// This must be called from within a function passed to suite().
export function beforeAll(f) {
  if (!curSuite) throw new Error('beforeAll() called outside suite()');
  curSuite.beforeAlls.push(f);
}

// Adds |f| to run after all tests in the suite.
// This must be called from within a function passed to suite().
export function afterAll(f) {
  if (!curSuite) throw new Error('afterAll() called outside suite()');
  curSuite.afterAlls.push(f);
}

// Adds |f| to run before each tests in the suite.
// This must be called from within a function passed to suite().
export function beforeEach(f) {
  if (!curSuite) throw new Error('beforeEach() called outside suite()');
  curSuite.beforeEaches.push(f);
}

// Adds |f| to run after all tests in the suite.
// This must be called from within a function passed to suite().
export function afterEach(f) {
  if (!curSuite) throw new Error('afterEach() called outside suite()');
  curSuite.afterEaches.push(f);
}

// Adds a test named |name| with function |f|.
// This must be called from within a function passed to suite().
// If |f| accepts an argument, it will be passed a 'done' callback that it
// must run upon completion. Additionally, |f| may be async.
export function test(name, f) {
  if (!curSuite) throw new Error('test() called outside suite()');
  curSuite.tests.push(new Test(name, f));
}

// Extracts filename, line, and column from a stack trace line:
// "    at ... (http://127.0.0.1:43559/test.js:101:15)"
// "    at http://127.0.0.1:43963/example.test.js:21:7"
const stackRegexp = /at .*\/([^/]+\.js):(\d+):(\d+)\)?$/;

// Tries to get filename and line (e.g. 'foo.test.js:123') from |err|.
// Uses the first stack frame not from this file.
function getSource(err) {
  if (!err.stack) return '';
  for (const ln of err.stack.split('\n')) {
    var matches = stackRegexp.exec(ln);
    if (matches && matches[1] !== 'test.js') {
      return `${matches[1]}:${matches[2]}`;
    }
  }
  // TODO: Maybe it was triggered by test.js.
  return '';
}

// Adds an error to |curResult| if we're in a test or |initResult| otherwise.
// |src| takes the form 'foo.test.js:23'.
function addError(msg, src) {
  const err = { msg };
  if (src) err.src = src;
  (curResult || initResult).errors.push(err);
}

// Adds an error describing |err|.
function handleException(err) {
  const src = getSource(err);
  console.error(`Exception from ${src || '[unknown]'}: ${err.toString()}`);
  const msg = err.toString() + ' (exception)';
  addError(msg, src);
}

// Adds an error describing |reason|.
function handleRejection(reason) {
  const src = getSource(new Error());
  console.error(`Unhandled rejection from ${src || '[unknown]'}: ${reason}`);
  addError(`Unhandled rejection: ${reason}`, src);
}

// Returns a result named |name| to return from runTests().
function newResult(name) {
  return { name, errors: [] };
}

// Fails the current test but continues running it.
export function error(msg) {
  const src = getSource(new Error());
  console.error(`Error from ${src}: ${msg}`);
  addError(msg, src);
}

// Fails the current test and aborts it.
export function fatal(msg) {
  throw new FatalError(msg);
}

// https://stackoverflow.com/a/32750746/6882947
class FatalError extends Error {
  constructor(msg) {
    super(msg);
    this.name = 'Fatal';
  }
}

function fmt(val) {
  return JSON.stringify(val);
}

// Adds an error if |got| doesn't strictly equal |want|.
// |desc| can contain a description of what's being compared.
export function expectEq(got, want, desc) {
  if (got !== want) {
    error(
      desc
        ? `${desc} is ${fmt(got)}; want ${fmt(want)}`
        : `Got ${fmt(got)}; want ${fmt(want)}`
    );
  }
}

// Errors in the window execution context, e.g. exceptions thrown from timeouts
// in either synchronous or async tests, trigger error events.
window.addEventListener('error', (ev) => {
  handleException(ev.error);
  if (lastDone) lastDone();
});

// Unhandled promise rejections trigger unhandledrejection events.
window.addEventListener('unhandledrejection', (ev) => {
  handleRejection(ev.reason);
  if (lastDone) lastDone();
});

// Runs all tests and returns results as an array of objects:
//
// {
//   name: 'suite.testName',
//   errors: [
//     {
//       msg: 'foo() = 3; want 4',
//       src: 'foo.test.js:23',
//     },
//     ...
//   ],
// }
//
// TODO: Take a test pattern?
export async function runTests() {
  results = [initResult];
  for (const s of allSuites) await s.runTests();
  return Promise.resolve(results);
}
