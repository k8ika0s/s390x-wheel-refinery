/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#0f172a",
        panel: "rgba(30, 41, 59, 0.6)",
        border: "rgba(51, 65, 85, 0.7)",
        accent: "#38bdf8",
        accent2: "#22c55e",
      },
      backdropBlur: {
        xs: "2px",
      },
    },
  },
  plugins: [],
};
