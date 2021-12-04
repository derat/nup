// Copyright 2021 Daniel Erat.
// All rights reserved.

// TODO: Decide if there's a cleaner way to do this. Right now, this stuff gets
// shoved into globals so it can be accessed by exported functions in an
// approximation of the style of other JS test frameworks.
const allSuites = []; // Suite objects registered via addSuite()
let curSuite = null; // Suite currently being added via addSuite()
let results = []; // test results from current run (see runTests())
let curResult = null; // in-progress test's result
let lastDone = null; // promise resolve func from last async test

// Suite contains a collection of tests.
class Suite {
  constructor(name) {
    this.name = name;
    this.tests = []; // Test objects
  }

  // Adds a test with name |name| and function |func| to the suite.
  addTest(name, func) {
    this.tests.push(new Test(name, func));
  }

  // Sequentially run all tests in the suite.
  // If |testName| is supplied, only the test with that name is run.
  async runTests(testName) {
    const tests = this.tests.filter(
      (t) => testName === undefined || t.name === testName
    );
    if (!tests.length) throw new Error(`No test ${this.name}.${testName}`);

    for (const t of tests) {
      const fullName = `${this.name}.${t.name}`;
      console.log(`Starting ${fullName}`);
      curResult = { name: fullName, errors: [] };
      await t.run();
      console.log(`Finished ${fullName}`);
      results.push(curResult);
      curResult = null;
    }
  }
}

// Test contains an individual test.
class Test {
  constructor(name, func) {
    this.name = name;
    this.func = func;
  }

  // Runs the test to completion.
  async run() {
    const handleException = (e) => {
      const src = getSource(e);
      console.error(`Exception from ${src}: ${e.toString()}`);
      addError(e.toString() + ' (exception)', src);
    };

    if (this.func.length === 1) {
      // This can't catch exceptions thrown from functions passed to
      // window.setTimeout():
      //  https://stackoverflow.com/q/41431605/6882947
      //  https://stackoverflow.com/q/60644708/6882947
      // Those get handled by window.onerror.
      await new Promise((resolve, reject) => {
        lastDone = resolve;
        this.func(resolve);
      }).catch((e) => {
        handleException(e);
      });
    } else {
      try {
        this.func();
      } catch (e) {
        handleException(e);
      }
    }
  }
}

// Adds a test suite named |name|.
// |func| is is executed immediately; it should call test() to define the
// suite's tests.
export function addSuite(name, f) {
  if (curSuite) throw new Error(`Already adding suite ${curSuite.name}`);
  curSuite = new Suite(name);
  f();
  if (!curSuite.tests.length) throw new Error('No tests defined');
  allSuites.push(curSuite);
  curSuite = null;
}

// Adds a test named |name| with function |func|.
// This must be called from within a function passed to addSuite().
export function test(name, f) {
  if (!curSuite) throw new Error('test() called outside addSuite()');
  curSuite.addTest(name, f);
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
  return '';
}

// Adds a test error to |curResult|.
// |src| takes the form 'foo.test.js:23'.
function addError(msg, src) {
  if (!curResult) throw new Error(`Can't add error outside of test`);
  const data = { msg };
  if (src) data.src = src;
  curResult.errors.push(data);
}

// Fails the current test but continues running it.
export function error(msg) {
  const src = getSource(new Error());
  console.error(`Error from ${src}: ${msg}`);
  addError(msg, src);
}

// Fails the current test and aborts it.
export function fatal(msg) {
  error(msg);
  throw new Error('Test aborted');
}

// Catch uncaught errors (e.g. exceptions thrown from setTimeout()).
window.onerror = (msg, source, line, col, err) => {
  const src = getSource(err);
  console.error(`Uncaught error from ${src}: ${err}`);
  if (curResult) {
    addError(`Uncaught exception: ${err}`, src);
    if (lastDone) lastDone();
  }
};

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
  results = [];
  for (const s of allSuites) await s.runTests();
  return Promise.resolve(results);
}
