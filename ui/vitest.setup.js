import "@testing-library/jest-dom";

// Quiet noisy router future warnings in tests
const warn = console.warn;
console.warn = (...args) => {
  if (typeof args[0] === "string" && args[0].includes("React Router Future Flag Warning")) {
    return;
  }
  warn(...args);
};
