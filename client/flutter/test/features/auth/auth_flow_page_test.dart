import 'package:arc/app/app.dart';
import 'package:arc/core/config/app_config.dart';
import 'package:arc/core/diagnostics/backend_probe.dart';
import 'package:arc/core/diagnostics/backend_status.dart';
import 'package:arc/features/auth/data/auth_repository.dart';
import 'package:arc/features/auth/data/providers.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('invite-only bootstrap lands on invite view', (tester) async {
    final repo = _FakeRepository(
      bootstrapResult: AuthBootstrapResult.unauthenticated(),
      inviteToken: '',
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          appConfigProvider.overrideWithValue(
            const AppConfig(
              apiBaseUrl: 'http://127.0.0.1:8080',
              inviteOnly: true,
              webCookieMode: true,
              csrfCookieName: 'arc_csrf_token',
              csrfHeaderName: 'X-CSRF-Token',
            ),
          ),
          authRepositoryProvider.overrideWithValue(repo),
          backendProbeProvider.overrideWith(
            (ref) => BackendProbeController.disabled(
              const BackendStatusState(
                level: BackendStatusLevel.online,
                message: 'Server reachable',
              ),
            ),
          ),
        ],
        child: const ArcApp(),
      ),
    );

    await tester.pump(const Duration(milliseconds: 900));

    expect(find.text('Enter Invite'), findsOneWidget);
    expect(find.byKey(const ValueKey('field_invite_token')), findsOneWidget);
  });

  testWidgets('authenticated bootstrap lands on authenticated view', (
    tester,
  ) async {
    final user = ArcUser(
      id: 'u1',
      username: 'user_one',
      email: 'user@example.com',
      displayName: 'User One',
      createdAt: DateTime.utc(2026, 1, 1),
    );

    final session = ArcSession(
      sessionId: 's1',
      accessToken: 'access',
      accessExpiresAt: DateTime.now().toUtc().add(const Duration(minutes: 15)),
      refreshToken: 'refresh',
      refreshExpiresAt: DateTime.now().toUtc().add(const Duration(days: 7)),
      rememberMe: true,
    );

    final repo = _FakeRepository(
      bootstrapResult: AuthBootstrapResult.authenticated(
        user: user,
        session: session,
      ),
      inviteToken: '',
      user: user,
      session: session,
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          appConfigProvider.overrideWithValue(
            const AppConfig(
              apiBaseUrl: 'http://127.0.0.1:8080',
              inviteOnly: true,
              webCookieMode: true,
              csrfCookieName: 'arc_csrf_token',
              csrfHeaderName: 'X-CSRF-Token',
            ),
          ),
          authRepositoryProvider.overrideWithValue(repo),
          backendProbeProvider.overrideWith(
            (ref) => BackendProbeController.disabled(
              const BackendStatusState(
                level: BackendStatusLevel.online,
                message: 'Server reachable',
              ),
            ),
          ),
        ],
        child: const ArcApp(),
      ),
    );

    await tester.pump(const Duration(milliseconds: 900));

    expect(find.text('Authenticated'), findsOneWidget);
    expect(find.textContaining('Welcome, User One'), findsOneWidget);
  });

  testWidgets('non invite-only bootstrap lands on login view', (tester) async {
    final repo = _FakeRepository(
      bootstrapResult: AuthBootstrapResult.unauthenticated(),
      inviteToken: '',
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          appConfigProvider.overrideWithValue(
            const AppConfig(
              apiBaseUrl: 'http://127.0.0.1:8080',
              inviteOnly: false,
              webCookieMode: false,
              csrfCookieName: 'arc_csrf_token',
              csrfHeaderName: 'X-CSRF-Token',
            ),
          ),
          authRepositoryProvider.overrideWithValue(repo),
          backendProbeProvider.overrideWith(
            (ref) => BackendProbeController.disabled(
              const BackendStatusState(
                level: BackendStatusLevel.online,
                message: 'Server reachable',
              ),
            ),
          ),
        ],
        child: const ArcApp(),
      ),
    );

    await tester.pump(const Duration(milliseconds: 900));

    expect(find.text('Sign In'), findsOneWidget);
    expect(find.text('Username'), findsWidgets);
    expect(find.text('Email'), findsWidgets);
  });
}

class _FakeRepository implements AuthRepository {
  _FakeRepository({
    required AuthBootstrapResult bootstrapResult,
    required String inviteToken,
    ArcUser? user,
    ArcSession? session,
  }) : _bootstrapResult = bootstrapResult,
       _inviteToken = inviteToken,
       _user = user,
       _session = session;

  final AuthBootstrapResult _bootstrapResult;
  String _inviteToken;
  ArcUser? _user;
  ArcSession? _session;

  @override
  ArcSession? get currentSession => _session;

  @override
  ArcUser? get currentUser => _user;

  @override
  Future<void> applyProfileDraft(ProfileDraft draft) async {}

  @override
  Future<AuthBootstrapResult> bootstrap() async => _bootstrapResult;

  @override
  Future<void> clearInviteToken() async {
    _inviteToken = '';
  }

  @override
  Future<void> clearProfileDraft() async {}

  @override
  Future<ArcInvite> createInvite({int maxUses = 1, Duration? ttl}) {
    throw UnimplementedError();
  }

  @override
  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    throw UnimplementedError();
  }

  @override
  Future<ArcSession> ensureActiveSession({
    Duration skew = Duration.zero,
  }) async {
    return _session!;
  }

  @override
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    throw UnimplementedError();
  }

  @override
  Future<void> logout() async {
    _user = null;
    _session = null;
  }

  @override
  Future<void> logoutAll() async {
    _user = null;
    _session = null;
  }

  @override
  Future<String?> readInviteToken() async {
    return _inviteToken;
  }

  @override
  Future<ArcSession> refreshSession() async {
    return _session!;
  }

  @override
  Future<ArcUser> refreshCurrentUser() async {
    return _user!;
  }

  @override
  Future<void> saveInviteToken(String token) async {
    _inviteToken = token;
  }

  @override
  Future<void> saveProfileDraft(ProfileDraft draft) async {}
}
