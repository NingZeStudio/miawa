import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import tseslint from '@typescript-eslint/eslint-plugin'
import tsParser from '@typescript-eslint/parser'
import vueParser from 'vue-eslint-parser'

export default [
  js.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  {
    files: ['src/**/*.{ts,vue}'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tsParser,
        sourceType: 'module',
        ecmaVersion: 'latest'
      }
    },
    plugins: {
      '@typescript-eslint': tseslint
    },
    rules: {
      'vue/multi-word-component-names': 'off',
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/no-explicit-any': 'off',
      'no-undef': 'off',
      'vue/no-v-html': 'off'
    }
  },
  {
    ignores: ['dist/**', 'node_modules/**', 'public/**', '*.cjs', '*.js', 'postcss.config.cjs', 'tailwind.config.cjs']
  }
]
