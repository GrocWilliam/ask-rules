import tseslint from 'typescript-eslint';
import eslintPluginSvelte from 'eslint-plugin-svelte';

export default [
  // Configuration de base pour TypeScript
  ...tseslint.configs.recommended,

  // Configuration pour Svelte
  ...eslintPluginSvelte.configs['flat/recommended'],

  // Configuration personnalisée
  {
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: ['.svelte'],
      },
    },
    rules: {
      // Règles TypeScript
      '@typescript-eslint/no-unused-vars': [
        'warn',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      '@typescript-eslint/no-explicit-any': 'warn',

      // Règles générales
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'prefer-const': 'warn',
    },
  },

  // Ignorer certains fichiers
  {
    ignores: [
      'node_modules/**',
      'build/**',
      'server/build/**',
      'dist/**',
      '.svelte-kit/**',
      'uploads/**',
      '*.config.js',
      '*.config.mjs',
      '*.config.ts',
    ],
  },
];
