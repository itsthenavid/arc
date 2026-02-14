import 'package:arc/core/config/app_config.dart';
import 'package:arc/features/auth/data/api_exception.dart';
import 'package:arc/features/auth/data/auth_repository.dart';
import 'package:arc/features/auth/data/providers.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

enum AuthView { bootstrapping, invite, signup, login, profile, authenticated }

class AuthState {
  const AuthState({
    required this.view,
    required this.inviteOnly,
    this.submitting = false,
    this.error,
    this.info,
    this.user,
    this.session,
    this.latestInvite,
    this.inviteToken = '',
    this.profileDraft = const ProfileDraft(),
  });

  final AuthView view;
  final bool inviteOnly;
  final bool submitting;
  final String? error;
  final String? info;
  final ArcUser? user;
  final ArcSession? session;
  final ArcInvite? latestInvite;
  final String inviteToken;
  final ProfileDraft profileDraft;

  bool get isAuthenticated => view == AuthView.authenticated;

  AuthState copyWith({
    AuthView? view,
    bool? submitting,
    Object? error = _notSet,
    Object? info = _notSet,
    Object? user = _notSet,
    Object? session = _notSet,
    Object? latestInvite = _notSet,
    String? inviteToken,
    Object? profileDraft = _notSet,
  }) {
    return AuthState(
      view: view ?? this.view,
      inviteOnly: inviteOnly,
      submitting: submitting ?? this.submitting,
      error: error == _notSet ? this.error : error as String?,
      info: info == _notSet ? this.info : info as String?,
      user: user == _notSet ? this.user : user as ArcUser?,
      session: session == _notSet ? this.session : session as ArcSession?,
      latestInvite: latestInvite == _notSet
          ? this.latestInvite
          : latestInvite as ArcInvite?,
      inviteToken: inviteToken ?? this.inviteToken,
      profileDraft: profileDraft == _notSet
          ? this.profileDraft
          : profileDraft as ProfileDraft,
    );
  }

  static const _notSet = Object();
}

final authControllerProvider = StateNotifierProvider<AuthController, AuthState>(
  (ref) {
    final repo = ref.watch(authRepositoryProvider);
    final cfg = ref.watch(appConfigProvider);
    return AuthController(repository: repo, config: cfg);
  },
);

class AuthController extends StateNotifier<AuthState> {
  AuthController({
    required AuthRepository repository,
    required AppConfig config,
  }) : _repository = repository,
       _config = config,
       super(
         AuthState(view: AuthView.bootstrapping, inviteOnly: config.inviteOnly),
       ) {
    bootstrap();
  }

  final AuthRepository _repository;
  final AppConfig _config;

  bool _bootstrapped = false;

  Future<void> bootstrap() async {
    if (_bootstrapped) {
      return;
    }
    _bootstrapped = true;

    state = state.copyWith(
      view: AuthView.bootstrapping,
      submitting: true,
      error: null,
      info: null,
    );

    try {
      final inviteToken = await _repository.readInviteToken() ?? '';
      final result = await _repository.bootstrap();

      if (result.authenticated &&
          result.user != null &&
          result.session != null) {
        state = state.copyWith(
          view: _resolvePostAuthView(result.user!, result.profileDraft),
          user: result.user,
          session: result.session,
          profileDraft: result.profileDraft ?? const ProfileDraft(),
          inviteToken: inviteToken,
          submitting: false,
          error: null,
        );
        return;
      }

      state = state.copyWith(
        view: _resolvePreAuthView(inviteToken),
        submitting: false,
        error: null,
        inviteToken: inviteToken,
        profileDraft: result.profileDraft ?? const ProfileDraft(),
      );
    } catch (error) {
      state = state.copyWith(
        submitting: false,
        view: _resolvePreAuthView(state.inviteToken),
        error: _humanizeError(error),
      );
    }
  }

  void clearMessages() {
    state = state.copyWith(error: null, info: null);
  }

  Future<void> submitInviteToken(String token) async {
    final normalized = token.trim();
    if (normalized.isEmpty) {
      state = state.copyWith(error: 'Invite token is required.');
      return;
    }

    await _repository.saveInviteToken(normalized);
    state = state.copyWith(
      inviteToken: normalized,
      view: AuthView.signup,
      error: null,
      info: null,
    );
  }

  void goToLogin() {
    state = state.copyWith(view: AuthView.login, error: null, info: null);
  }

  void goToInvite() {
    state = state.copyWith(view: AuthView.invite, error: null, info: null);
  }

  void goToSignup() {
    state = state.copyWith(view: AuthView.signup, error: null, info: null);
  }

  Future<void> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      final bundle = await _repository.login(
        username: username,
        email: email,
        password: password,
        rememberMe: rememberMe,
      );

      state = state.copyWith(
        submitting: false,
        user: bundle.user,
        session: bundle.session,
        latestInvite: null,
        profileDraft: const ProfileDraft(),
        view: _resolvePostAuthView(bundle.user, null),
        error: null,
        info: null,
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  Future<void> signupWithInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      final bundle = await _repository.consumeInvite(
        inviteToken: inviteToken,
        username: username,
        email: email,
        password: password,
        rememberMe: rememberMe,
      );

      state = state.copyWith(
        submitting: false,
        user: bundle.user,
        session: bundle.session,
        latestInvite: null,
        profileDraft: const ProfileDraft(),
        view: _resolvePostAuthView(bundle.user, null),
        error: null,
        info: null,
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  Future<void> completeProfile(ProfileDraft draft) async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      await _repository.applyProfileDraft(draft);
      await _repository.clearProfileDraft();

      final existing = state.user;
      final updated = existing?.copyWith(
        displayName: _trimOrNull(draft.displayName) ?? existing.displayName,
        username: _trimOrNull(draft.username) ?? existing.username,
        bio: _trimOrNull(draft.bio) ?? existing.bio,
      );

      state = state.copyWith(
        submitting: false,
        user: updated,
        profileDraft: draft,
        view: AuthView.authenticated,
        info: 'Profile details were saved locally for this phase.',
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  Future<void> refreshUser() async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      final session = await _repository.ensureActiveSession(
        skew: const Duration(seconds: 10),
      );
      final user = await _repository.refreshCurrentUser();
      state = state.copyWith(
        submitting: false,
        user: user,
        session: session,
        info: 'Session refreshed.',
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  Future<void> logout({required bool all}) async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      if (all) {
        await _repository.logoutAll();
      } else {
        await _repository.logout();
      }

      final inviteToken = await _repository.readInviteToken() ?? '';
      state = state.copyWith(
        submitting: false,
        user: null,
        session: null,
        latestInvite: null,
        profileDraft: const ProfileDraft(),
        view: _resolvePreAuthView(inviteToken),
        error: null,
        info: all ? 'Logged out of all sessions.' : 'Logged out.',
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  Future<void> createInvite({int maxUses = 1, Duration? ttl}) async {
    state = state.copyWith(submitting: true, error: null, info: null);

    try {
      final invite = await _repository.createInvite(maxUses: maxUses, ttl: ttl);
      state = state.copyWith(
        submitting: false,
        latestInvite: invite,
        info: 'Invite created successfully.',
      );
    } catch (e) {
      state = state.copyWith(submitting: false, error: _humanizeError(e));
    }
  }

  AuthView _resolvePreAuthView(String inviteToken) {
    if (_config.inviteOnly) {
      return inviteToken.trim().isNotEmpty ? AuthView.signup : AuthView.invite;
    }
    return AuthView.login;
  }

  AuthView _resolvePostAuthView(ArcUser user, ProfileDraft? draft) {
    if (user.needsProfileCompletion) {
      return AuthView.profile;
    }
    if (draft != null && !draft.isEmpty) {
      return AuthView.profile;
    }
    return AuthView.authenticated;
  }

  String _humanizeError(Object error) {
    if (error is ApiException) {
      switch (error.kind) {
        case ApiErrorKind.network:
          return 'Cannot reach server. Check backend status and API URL.';
        case ApiErrorKind.timeout:
          return 'Request timed out. Please retry.';
        case ApiErrorKind.unavailable:
          return 'Service unavailable right now. Please try again.';
        case ApiErrorKind.invalidResponse:
          return 'Server returned malformed response.';
        default:
          break;
      }

      switch (error.code) {
        case 'invalid_credentials':
          return 'Invalid credentials.';
        case 'invalid_invite':
          return 'The invite token is invalid or expired.';
        case 'conflict':
          return 'Username or email is already in use.';
        case 'captcha_invalid':
          return 'Captcha verification failed.';
        case 'refresh_reuse_detected':
          return 'Session security event detected. Please log in again.';
        case 'csrf_invalid':
          return 'CSRF validation failed. Refresh the page and retry.';
        case 'db_unavailable':
          return 'Server is running without database/auth mode.';
        case 'server_busy':
          return 'Server is busy. Please retry shortly.';
        default:
          if (error.message.trim().isNotEmpty) {
            return error.message.trim();
          }
          return 'Request failed. Please try again.';
      }
    }
    return 'Request failed. Please try again.';
  }

  String? _trimOrNull(String? value) {
    if (value == null) {
      return null;
    }
    final trimmed = value.trim();
    if (trimmed.isEmpty) {
      return null;
    }
    return trimmed;
  }
}
