/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/**/*.{js,ts,jsx,tsx,vue}',
    './node_modules/primevue/**/*.{vue,js,ts,jsx,tsx}'
  ],
  theme: {
    extend: {
      colors: {
        'main-color': '#E4E6E5',
        'main-bg': '#FAFAFA'
      },
      textColor: {
        'panel-caption': '#565757'
      },
      height: {
        'control-panel': '8rem'
      }
    }
  },
  plugins: []
}
