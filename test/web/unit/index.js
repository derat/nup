// Copyright 2021 Daniel Erat.
// All rights reserved.

import { runTests } from './test.js';

// TODO: Is there a cleaner way to make an exported function from an ES6 module
// callable via Selenium?
window.runTests = runTests;
